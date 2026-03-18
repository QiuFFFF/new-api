package controller

import (
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/QuantumNous/new-api/setting/operation_setting"

	"github.com/gin-gonic/gin"
)

// GetAdminMonitoringGroups returns all monitoring group stats for admin
func GetAdminMonitoringGroups(c *gin.Context) {
	setting := operation_setting.GetGroupMonitoringSetting()
	monitoringGroups := setting.MonitoringGroups
	if len(monitoringGroups) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data":    []interface{}{},
		})
		return
	}

	stats, err := model.GetGroupMonitoringStatsByNames(monitoringGroups)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "获取监控数据失败: " + err.Error(),
		})
		return
	}

	// Order by GroupDisplayOrder if set
	orderedStats := orderGroupStats(stats, setting.GroupDisplayOrder)

	// Add computed fields for frontend
	enrichedStats := make([]gin.H, 0, len(orderedStats))
	for _, s := range orderedStats {
		enrichedStats = append(enrichedStats, gin.H{
			"id":                s.Id,
			"group_name":        s.GroupName,
			"availability_rate": s.AvailabilityRate,
			"cache_hit_rate":    s.CacheHitRate,
			"avg_response_time": s.AvgResponseTime,
			"avg_frt":           s.AvgFRT,
			"online_channels":   s.OnlineChannels,
			"total_channels":    s.TotalChannels,
			"group_ratio":       s.GroupRatio,
			"last_test_model":   s.LastTestModel,
			"updated_at":        s.UpdatedAt,
			"has_traffic":       s.AvailabilityRate >= 0 || s.CacheHitRate >= 0,
			"is_online":         s.AvailabilityRate >= 90 || (s.AvailabilityRate < 0 && s.CacheHitRate >= 0),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    enrichedStats,
	})
}

// GetAdminMonitoringGroupDetail returns detailed monitoring info for a single group (admin)
func GetAdminMonitoringGroupDetail(c *gin.Context) {
	groupName := c.Param("group")
	if groupName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "分组名称不能为空",
		})
		return
	}

	groupStat, err := model.GetGroupMonitoringStatByName(groupName)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "分组不存在或无监控数据",
		})
		return
	}

	channelStats, err := model.GetChannelMonitoringStatsByGroup(groupName)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "获取渠道监控数据失败: " + err.Error(),
		})
		return
	}

	// Filter out deleted channels: only keep stats for currently active channels
	activeChannels, err := model.GetAllChannelsByGroup(groupName)
	if err == nil {
		activeSet := make(map[int]bool, len(activeChannels))
		channelNameMap := make(map[int]string, len(activeChannels))
		channelStatusMap := make(map[int]int, len(activeChannels))
		for _, ch := range activeChannels {
			activeSet[ch.Id] = true
			channelNameMap[ch.Id] = ch.Name
			channelStatusMap[ch.Id] = ch.Status
		}
		filtered := make([]model.ChannelMonitoringStat, 0, len(channelStats))
		seenChannels := make(map[int]bool, len(channelStats))
		for _, cs := range channelStats {
			if activeSet[cs.ChannelId] {
				cs.ChannelName = channelNameMap[cs.ChannelId]
				cs.ChannelStatus = channelStatusMap[cs.ChannelId]
				filtered = append(filtered, cs)
				seenChannels[cs.ChannelId] = true
			}
		}
		// Add channels that exist but have no monitoring stats (e.g., disabled channels)
		for _, ch := range activeChannels {
			if !seenChannels[ch.Id] {
				filtered = append(filtered, model.ChannelMonitoringStat{
					GroupName:        groupName,
					ChannelId:        ch.Id,
					ChannelName:      ch.Name,
					ChannelStatus:    ch.Status,
					AvailabilityRate: -1,
					CacheHitRate:     -1,
				})
			}
		}
		channelStats = filtered
	}

	c.JSON(http.StatusOK, gin.H{
		"success":       true,
		"message":       "",
		"data":          groupStat,
		"channel_stats": channelStats,
	})
}

// GetAdminMonitoringGroupHistory returns history data for charts (admin)
func GetAdminMonitoringGroupHistory(c *gin.Context) {
	groupName := c.Param("group")
	if groupName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "分组名称不能为空",
		})
		return
	}

	setting := operation_setting.GetGroupMonitoringSetting()
	endTime := time.Now().Unix()
	periodMinutes := setting.AvailabilityPeriodMinutes
	if setting.CacheHitPeriodMinutes > periodMinutes {
		periodMinutes = setting.CacheHitPeriodMinutes
	}
	startTime := endTime - int64(periodMinutes*60)

	history, err := model.GetMonitoringHistory(groupName, startTime, endTime)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "获取历史数据失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":                      true,
		"message":                      "",
		"data":                         history,
		"period_minutes":               periodMinutes,
		"aggregation_interval_minutes": setting.AggregationIntervalMinutes,
	})
}

// RefreshMonitoringData triggers an immediate re-aggregation (admin)
func RefreshMonitoringData(c *gin.Context) {
	ok := service.TriggerAggregationRefresh()
	if !ok {
		c.JSON(http.StatusTooManyRequests, gin.H{
			"success": false,
			"message": "聚合正在运行中，请稍后再试",
		})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{
		"success": true,
		"message": "刷新已触发，数据将在几秒后更新",
	})
}

// DeleteMonitoringGroupRecords clears all monitoring data for a group (admin)
func DeleteMonitoringGroupRecords(c *gin.Context) {
	groupName := c.Param("group")
	if groupName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "分组名称不能为空",
		})
		return
	}

	totalDeleted, err := model.DeleteAllMonitoringDataForGroup(groupName)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "清空记录失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "清空成功",
		"data": gin.H{
			"deleted_rows": totalDeleted,
		},
	})
}

// GetPublicMonitoringGroups returns monitoring data for public/user view
func GetPublicMonitoringGroups(c *gin.Context) {
	setting := operation_setting.GetGroupMonitoringSetting()

	monitoringGroups := setting.MonitoringGroups
	if len(monitoringGroups) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"message": "",
			"data":    []interface{}{},
		})
		return
	}

	stats, err := model.GetGroupMonitoringStatsForPublic(monitoringGroups)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "获取监控数据失败",
		})
		return
	}

	// Desensitize: only return group-level info
	desensitized := make([]gin.H, 0, len(stats))
	for _, s := range stats {
		desensitized = append(desensitized, desensitizeGroupStat(&s))
	}

	orderedData := orderDesensitizedStats(desensitized, setting.GroupDisplayOrder)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "",
		"data":    orderedData,
	})
}

// GetPublicMonitoringGroupHistory returns history data for charts (public/user)
func GetPublicMonitoringGroupHistory(c *gin.Context) {
	setting := operation_setting.GetGroupMonitoringSetting()

	groupName := c.Param("group")
	if groupName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "分组名称不能为空",
		})
		return
	}

	// Verify group is in the monitored list
	monitoringGroups := setting.MonitoringGroups
	found := false
	for _, g := range monitoringGroups {
		if g == groupName {
			found = true
			break
		}
	}
	if !found {
		c.JSON(http.StatusForbidden, gin.H{
			"success": false,
			"message": "该分组不在监控列表中",
		})
		return
	}

	endTime := time.Now().Unix()
	periodMinutes := setting.AvailabilityPeriodMinutes
	if setting.CacheHitPeriodMinutes > periodMinutes {
		periodMinutes = setting.CacheHitPeriodMinutes
	}
	startTime := endTime - int64(periodMinutes*60)

	history, err := model.GetMonitoringHistory(groupName, startTime, endTime)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"success": false,
			"message": "获取历史数据失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":                      true,
		"message":                      "",
		"data":                         history,
		"period_minutes":               periodMinutes,
		"aggregation_interval_minutes": setting.AggregationIntervalMinutes,
	})
}

// desensitizeGroupStat removes sensitive info from group stat for public view
func desensitizeGroupStat(stat *model.GroupMonitoringStat) gin.H {
	return gin.H{
		"group_name":        stat.GroupName,
		"availability_rate": stat.AvailabilityRate,
		"cache_hit_rate":    stat.CacheHitRate,
		"avg_response_time": stat.AvgResponseTime,
		"avg_frt":           stat.AvgFRT,
		"is_online":         stat.AvailabilityRate >= 90 || (stat.AvailabilityRate < 0 && stat.CacheHitRate >= 0),
		"has_traffic":       stat.AvailabilityRate >= 0 || stat.CacheHitRate >= 0,
		"group_ratio":       stat.GroupRatio,
		"last_test_model":   stat.LastTestModel,
		"updated_at":        stat.UpdatedAt,
	}
}

// orderGroupStats orders stats by GroupDisplayOrder
func orderGroupStats(stats []model.GroupMonitoringStat, order []string) []model.GroupMonitoringStat {
	if len(order) == 0 {
		return stats
	}

	orderMap := make(map[string]int)
	for i, name := range order {
		orderMap[name] = i
	}

	ordered := make([]model.GroupMonitoringStat, 0, len(stats))
	remaining := make([]model.GroupMonitoringStat, 0)

	// First add ordered ones
	statMap := make(map[string]model.GroupMonitoringStat)
	for _, s := range stats {
		statMap[s.GroupName] = s
	}

	for _, name := range order {
		if s, ok := statMap[name]; ok {
			ordered = append(ordered, s)
			delete(statMap, name)
		}
	}

	// Then add remaining
	for _, s := range statMap {
		remaining = append(remaining, s)
	}

	return append(ordered, remaining...)
}

// orderDesensitizedStats orders desensitized stats by GroupDisplayOrder
func orderDesensitizedStats(stats []gin.H, order []string) []gin.H {
	if len(order) == 0 {
		return stats
	}

	orderMap := make(map[string]int)
	for i, name := range order {
		orderMap[name] = i
	}

	statMap := make(map[string]gin.H)
	for _, s := range stats {
		if name, ok := s["group_name"].(string); ok {
			statMap[name] = s
		}
	}

	ordered := make([]gin.H, 0, len(stats))
	for _, name := range order {
		if s, ok := statMap[name]; ok {
			ordered = append(ordered, s)
			delete(statMap, name)
		}
	}

	for _, s := range statMap {
		ordered = append(ordered, s)
	}

	return ordered
}
