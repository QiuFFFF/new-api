package service

import (
	"math"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"
)

// carryForwardFromPrev applies a small random ±2 offset to the previous rate for display
// when no new requests exist at the channel level. Clamped to [0, 100].
// Returns -1 if prevRate < 0 (no previous data).
func carryForwardFromPrev(prevRate float64) float64 {
	if prevRate < 0 {
		return -1
	}
	offset := float64(rand.Intn(5) - 2) // -2 ~ +2
	v := math.Round((prevRate+offset)*100) / 100
	if v < 0 {
		v = 0
	}
	if v > 100 {
		v = 100
	}
	return v
}

// carryForwardFromHistory takes the average of the most recent 10 history records
// and applies a small random ±2 offset. Used for group-level and history-level carry-forward.
// Returns -1 if no historical data exists.
func carryForwardFromHistory(groupName string, field string) float64 {
	var avg float64
	if field == "availability" {
		avg = model.GetRecentAvailabilityAvg(groupName, 10)
	} else {
		avg = model.GetRecentCacheHitAvg(groupName, 10)
	}
	if avg < 0 {
		return -1
	}
	offset := float64(rand.Intn(5) - 2)
	v := math.Round((avg+offset)*100) / 100
	if v < 0 {
		v = 0
	}
	if v > 100 {
		v = 100
	}
	return v
}

var (
	groupMonitoringOnce    sync.Once
	aggregationRunning     atomic.Int32
)

// StartGroupMonitoringAggregation starts the background aggregation loop
func StartGroupMonitoringAggregation() {
	if !common.IsMasterNode {
		return
	}
	groupMonitoringOnce.Do(func() {
		// Wait for DB to be ready
		time.Sleep(10 * time.Second)

		// Run initial aggregation — use fullRefresh if no historical data exists
		// so that the first startup backfills availability stats from recent logs.
		if model.IsRequestStatsEmpty() {
			// Wrap in anonymous func so defer guarantees aggregationRunning reset
			// even if runAggregationCycle panics during the initial backfill.
			if aggregationRunning.CompareAndSwap(0, 1) {
				func() {
					defer aggregationRunning.Store(0)
					runAggregationCycle(true)
				}()
			}
		} else {
			runAggregationCycleSafe()
		}

		for {
			setting := operation_setting.GetGroupMonitoringSetting()
			intervalMinutes := setting.AggregationIntervalMinutes
			if intervalMinutes < 1 {
				intervalMinutes = 5
			}

			if len(setting.MonitoringGroups) == 0 {
				// No groups configured, sleep and retry
				time.Sleep(1 * time.Minute)
				continue
			}

			time.Sleep(time.Duration(intervalMinutes) * time.Minute)
			runAggregationCycleSafe()
		}
	})
}

// TriggerAggregationRefresh triggers an immediate refresh (clears all monitoring data and re-aggregates)
func TriggerAggregationRefresh() bool {
	if !aggregationRunning.CompareAndSwap(0, 1) {
		return false // already running
	}
	go func() {
		defer aggregationRunning.Store(0)

		// Clear all monitoring data before re-aggregation
		model.DeleteAllMonitoringData()

		runAggregationCycle(true)
	}()
	return true
}

func runAggregationCycleSafe() {
	if !aggregationRunning.CompareAndSwap(0, 1) {
		return // already running
	}
	defer aggregationRunning.Store(0)
	runAggregationCycle(false)
}

func runAggregationCycle(fullRefresh bool) {
	setting := operation_setting.GetGroupMonitoringSetting()
	monitoringGroups := setting.MonitoringGroups
	if len(monitoringGroups) == 0 {
		return
	}

	now := time.Now().Unix()
	intervalMinutes := setting.AggregationIntervalMinutes
	if intervalMinutes < 1 {
		intervalMinutes = 5
	}
	intervalSeconds := int64(intervalMinutes * 60)

	// Step 1: Determine time windows
	availPeriod := int64(setting.AvailabilityPeriodMinutes * 60)
	if availPeriod < 300 {
		availPeriod = 3600 // default 60 min
	}
	cachePeriod := int64(setting.CacheHitPeriodMinutes * 60)
	if cachePeriod < 300 {
		cachePeriod = 3600
	}

	// Use the larger period for full-refresh log query window
	queryPeriod := availPeriod
	if cachePeriod > queryPeriod {
		queryPeriod = cachePeriod
	}

	// Normal cycle: only scan the last interval of logs (already aggregated data lives in request_stats)
	// Full refresh: scan the entire period
	var startTime int64
	if fullRefresh {
		startTime = now - queryPeriod
	} else {
		startTime = now - intervalSeconds
	}
	endTime := now

	// Step 2: Query LOG_DB and aggregate logs
	rows, err := model.AggregateLogsForMonitoring(
		startTime,
		endTime,
		intervalSeconds,
		setting.AvailabilityExcludeModels,
		setting.CacheHitExcludeModels,
		setting.AvailabilityExcludeKeywords,
	)
	if err != nil {
		common.SysError("group monitoring: failed to aggregate logs: " + err.Error())
		return
	}

	// Step 3: Convert to RequestStat and upsert
	stats := make([]model.RequestStat, len(rows))
	for i, row := range rows {
		stats[i] = model.RequestStat{
			BucketStart:       row.BucketStart,
			GroupName:         row.GroupName,
			ChannelId:         row.ChannelId,
			ModelName:         row.ModelName,
			TotalRequests:     row.TotalRequests,
			SuccessRequests:   row.SuccessRequests,
			ErrorRequests:     row.ErrorRequests,
			TotalCacheTokens:  row.TotalCacheTokens,
			TotalPromptTokens: row.TotalPromptTokens,
			CacheDataPoints:   row.CacheDataPoints,
			SumResponseTime:   row.SumResponseTime,
		}
	}
	if err := model.UpsertRequestStats(stats); err != nil {
		common.SysError("group monitoring: failed to upsert request stats: " + err.Error())
		return
	}

	// Step 4a: Aggregate availability stats (with availability excludes and period)
	availStart := now - availPeriod
	availAggs, err := model.AggregateAvailabilityByGroupChannel(availStart, setting.AvailabilityExcludeModels)
	if err != nil {
		common.SysError("group monitoring: failed to aggregate availability stats: " + err.Error())
		return
	}

	// Step 4b: Aggregate cache hit stats (with cache excludes and period)
	cacheStart := now - cachePeriod
	cacheAggs, err := model.AggregateCacheHitByGroupChannel(cacheStart, setting.CacheHitExcludeModels)
	if err != nil {
		common.SysError("group monitoring: failed to aggregate cache hit stats: " + err.Error())
		return
	}

	// Build maps: group+channel -> stats
	type channelKey struct {
		GroupName string
		ChannelId int
	}
	availMap := make(map[channelKey]*model.AvailabilityAggRow)
	for i := range availAggs {
		key := channelKey{availAggs[i].GroupName, availAggs[i].ChannelId}
		availMap[key] = &availAggs[i]
	}
	cacheMap := make(map[channelKey]*model.CacheHitAggRow)
	for i := range cacheAggs {
		key := channelKey{cacheAggs[i].GroupName, cacheAggs[i].ChannelId}
		cacheMap[key] = &cacheAggs[i]
	}

	// Step 4c: Per-interval aggregation for history chart
	// fullRefresh=true: aggregate all buckets in the availability period for backfill
	// fullRefresh=false: aggregate only the last interval
	type bucketGroupKey struct {
		BucketStart int64
		GroupName   string
	}

	bucketAvailMap := make(map[bucketGroupKey]*model.GroupAvailabilityBucketRow)
	bucketCacheMap := make(map[bucketGroupKey]*model.GroupCacheHitBucketRow)
	intervalAvailMap := make(map[string]*model.GroupAvailabilityAggRow)
	intervalCacheMap := make(map[string]*model.GroupCacheHitAggRow)

	if fullRefresh {
		// Aggregate all buckets across the entire availability period
		bucketAvailAggs, err := model.AggregateAvailabilityByGroupBucket(availStart, setting.AvailabilityExcludeModels)
		if err != nil {
			common.SysError("group monitoring: failed to aggregate bucket availability: " + err.Error())
		} else {
			for i := range bucketAvailAggs {
				key := bucketGroupKey{bucketAvailAggs[i].BucketStart, bucketAvailAggs[i].GroupName}
				bucketAvailMap[key] = &bucketAvailAggs[i]
			}
		}
		bucketCacheAggs, err := model.AggregateCacheHitByGroupBucket(cacheStart, setting.CacheHitExcludeModels)
		if err != nil {
			common.SysError("group monitoring: failed to aggregate bucket cache hit: " + err.Error())
		} else {
			for i := range bucketCacheAggs {
				key := bucketGroupKey{bucketCacheAggs[i].BucketStart, bucketCacheAggs[i].GroupName}
				bucketCacheMap[key] = &bucketCacheAggs[i]
			}
		}
	} else {
		// Normal cycle: only aggregate the last interval
		intervalStart := now - intervalSeconds
		intervalAvailAggs, err := model.AggregateAvailabilityByGroup(intervalStart, setting.AvailabilityExcludeModels)
		if err != nil {
			common.SysError("group monitoring: failed to aggregate interval availability: " + err.Error())
		}
		intervalCacheAggs, err := model.AggregateCacheHitByGroup(intervalStart, setting.CacheHitExcludeModels)
		if err != nil {
			common.SysError("group monitoring: failed to aggregate interval cache hit: " + err.Error())
		}

		for i := range intervalAvailAggs {
			intervalAvailMap[intervalAvailAggs[i].GroupName] = &intervalAvailAggs[i]
		}
		for i := range intervalCacheAggs {
			intervalCacheMap[intervalCacheAggs[i].GroupName] = &intervalCacheAggs[i]
		}
	}

	// Step 5: Get channel test info
	channelTestInfos, err := model.GetChannelTestInfoByGroups(monitoringGroups)
	if err != nil {
		common.SysError("group monitoring: failed to get channel test info: " + err.Error())
		return
	}

	// Build channel test info map
	// A channel's Group field may be "group1,group2,group3", so map each individual group to the info
	testInfoMap := make(map[channelKey]*model.ChannelTestInfo)
	for i := range channelTestInfos {
		info := &channelTestInfos[i]
		for _, g := range strings.Split(info.Group, ",") {
			g = strings.TrimSpace(g)
			if g != "" {
				testInfoMap[channelKey{g, info.ChannelId}] = info
			}
		}
	}

	// Step 6: Upsert channel_monitoring_stats and calculate group stats
	var historyRecords []model.MonitoringHistory

	// Build set for cache-tokens-separate groups (Claude-style: prompt_tokens excludes cache)
	cacheSeparateSet := make(map[string]bool)
	for _, g := range setting.CacheTokensSeparateGroups {
		cacheSeparateSet[g] = true
	}

	for _, groupName := range monitoringGroups {
		var groupTotalRequests int
		var groupSuccessRequests int
		var groupTotalCacheTokens int64
		var groupTotalPromptTokens int64
		var groupCacheDataPoints int
		var groupSumResponseTime int64
		var onlineChannels int
		var totalChannels int
		var lastTestModel string
		isCacheSeparate := cacheSeparateSet[groupName]

		// Get all enabled channels in this group
		channels, err := model.GetChannelsByGroup(groupName)
		if err != nil {
			common.SysError("group monitoring: failed to get channels for group " + groupName + ": " + err.Error())
			continue
		}
		totalChannels = len(channels)

		// Fetch previous channel stats for carry-forward when a channel has no requests
		prevChannelStats, _ := model.GetChannelMonitoringStatsByGroup(groupName)
		prevChannelStatMap := make(map[int]*model.ChannelMonitoringStat, len(prevChannelStats))
		for i := range prevChannelStats {
			prevChannelStatMap[prevChannelStats[i].ChannelId] = &prevChannelStats[i]
		}

		// Get latest FRT for each channel in this group
		channelIds := make([]int, 0, len(channels))
		for _, ch := range channels {
			channelIds = append(channelIds, ch.Id)
		}
		frtMap, frtErr := model.GetLatestFRTForChannels(availStart, groupName, channelIds)
		if frtErr != nil {
			common.SysError("group monitoring: failed to get FRT for group " + groupName + ": " + frtErr.Error())
			frtMap = nil
		}

		// Determine online/offline based on latest log status (not channel test)
		// Only use AvailabilityExcludeModels — cache excludes are unrelated to online status
		onlineMap, onlineErr := model.GetLatestLogStatusForChannels(availStart, groupName, channelIds, setting.AvailabilityExcludeModels, setting.AvailabilityExcludeKeywords)
		if onlineErr != nil {
			common.SysError("group monitoring: failed to get online status for group " + groupName + ": " + onlineErr.Error())
			onlineMap = nil
		}

		// Get latest model_name for channels without TestModel configured
		modelMap, modelErr := model.GetLatestModelForChannels(groupName, channelIds)
		if modelErr != nil {
			common.SysError("group monitoring: failed to get latest model for group " + groupName + ": " + modelErr.Error())
			modelMap = nil
		}

		for _, ch := range channels {
			key := channelKey{groupName, ch.Id}
			availAgg := availMap[key]
			cacheAgg := cacheMap[key]
			testInfo := testInfoMap[key]

			var availRate float64 = -1
			var cacheHitRate float64 = -1
			var responseTime int
			var testTime int64
			testModel := ""
			isOnline := false

			if availAgg != nil {
				totalReq := availAgg.TotalRequests
				if totalReq > 0 {
					availRate = float64(availAgg.SuccessRequests) / float64(totalReq) * 100
				}
				groupTotalRequests += availAgg.TotalRequests
				groupSuccessRequests += availAgg.SuccessRequests
				groupSumResponseTime += availAgg.SumResponseTime
			}

			if cacheAgg != nil {
				if cacheAgg.CacheDataPoints > 0 {
					var totalTokens int64
					if isCacheSeparate {
						// Claude-style: prompt_tokens does NOT include cache_tokens
						totalTokens = cacheAgg.TotalPromptTokens + cacheAgg.TotalCacheTokens
					} else {
						// OpenAI-style: prompt_tokens already includes cache_tokens
						totalTokens = cacheAgg.TotalPromptTokens
					}
					if totalTokens > 0 {
						cacheHitRate = float64(cacheAgg.TotalCacheTokens) / float64(totalTokens) * 100
						if cacheHitRate > 100 {
							cacheHitRate = 100
						}
					}
				}
				groupTotalCacheTokens += cacheAgg.TotalCacheTokens
				groupTotalPromptTokens += cacheAgg.TotalPromptTokens
				groupCacheDataPoints += cacheAgg.CacheDataPoints
			}

			// No requests for this channel — carry forward previous stat
			if availRate < 0 || cacheHitRate < 0 {
				if prev, ok := prevChannelStatMap[ch.Id]; ok {
					if availRate < 0 {
						availRate = carryForwardFromPrev(prev.AvailabilityRate)
					}
					if cacheHitRate < 0 {
						cacheHitRate = carryForwardFromPrev(prev.CacheHitRate)
					}
				}
			}

			if testInfo != nil {
				responseTime = testInfo.ResponseTime
				testTime = testInfo.TestTime
				testModel = testInfo.TestModel
			}

			// Fallback: if TestModel is not configured, use latest model from logs
			if testModel == "" && modelMap != nil {
				if m, ok := modelMap[ch.Id]; ok {
					testModel = m
				}
			}

			// Determine online status from latest log entry
			if onlineMap != nil {
				isOnline = onlineMap[ch.Id]
			}

			if isOnline {
				onlineChannels++
			}
			if testModel != "" {
				lastTestModel = testModel
			}

			// Upsert channel monitoring stat
			lastFRT := 0
			if frtMap != nil {
				if v, ok := frtMap[ch.Id]; ok {
					lastFRT = v
				}
			}
			stat := &model.ChannelMonitoringStat{
				GroupName:        groupName,
				ChannelId:        ch.Id,
				AvailabilityRate: availRate,
				CacheHitRate:     cacheHitRate,
				LastResponseTime: responseTime,
				LastFRT:          lastFRT,
				LastTestTime:     testTime,
				LastTestModel:    testModel,
				IsOnline:         isOnline,
				UpdatedAt:        now,
			}
			if err := model.UpsertChannelMonitoringStat(stat); err != nil {
				common.SysError("group monitoring: failed to upsert channel stat: " + err.Error())
			}
		}

		// Clean up orphan channel monitoring stats (deleted channels)
		if len(channels) > 0 {
			activeChannelIds := make([]int, 0, len(channels))
			for _, ch := range channels {
				activeChannelIds = append(activeChannelIds, ch.Id)
			}
			model.DeleteOrphanChannelMonitoringStats(groupName, activeChannelIds)
		} else {
			// No active channels — delete all channel stats for this group
			model.DeleteChannelMonitoringStatsByGroup(groupName)
		}

		// Calculate group-level stats
		var groupAvailRate float64 = -1
		var groupCacheHitRate float64 = -1
		var avgResponseTime int

		if groupTotalRequests > 0 {
			groupAvailRate = float64(groupSuccessRequests) / float64(groupTotalRequests) * 100
		} else {
			// No requests in this period — carry forward from historical average
			groupAvailRate = carryForwardFromHistory(groupName, "availability")
		}
		if groupCacheDataPoints > 0 {
			var totalTokens int64
			if isCacheSeparate {
				totalTokens = groupTotalPromptTokens + groupTotalCacheTokens
			} else {
				totalTokens = groupTotalPromptTokens
			}
			if totalTokens > 0 {
				groupCacheHitRate = float64(groupTotalCacheTokens) / float64(totalTokens) * 100
				if groupCacheHitRate > 100 {
					groupCacheHitRate = 100
				}
			}
		} else if groupTotalRequests == 0 {
			// No cache data either — carry forward from historical average
			groupCacheHitRate = carryForwardFromHistory(groupName, "cache")
		}
		if groupTotalRequests > 0 {
			avgResponseTime = int(groupSumResponseTime / int64(groupTotalRequests))
		}

		// Calculate group-level average FRT from channel FRTs
		var avgFRT int
		if frtMap != nil && len(frtMap) > 0 {
			var frtSum, frtCount int
			for _, frt := range frtMap {
				frtSum += frt
				frtCount++
			}
			if frtCount > 0 {
				avgFRT = frtSum / frtCount
			}
		}

		groupRatio := ratio_setting.GetGroupRatio(groupName)

		groupStat := &model.GroupMonitoringStat{
			GroupName:        groupName,
			AvailabilityRate: groupAvailRate,
			CacheHitRate:     groupCacheHitRate,
			AvgResponseTime:  avgResponseTime,
			AvgFRT:           avgFRT,
			OnlineChannels:   onlineChannels,
			TotalChannels:    totalChannels,
			GroupRatio:       groupRatio,
			LastTestModel:    lastTestModel,
			UpdatedAt:        now,
		}
		if err := model.UpsertGroupMonitoringStat(groupStat); err != nil {
			common.SysError("group monitoring: failed to upsert group stat: " + err.Error())
		}

		// Step 7: Insert monitoring history
		if fullRefresh {
			// Backfill: generate one history record per (bucket_start, group) combination
			// Collect all bucket timestamps relevant to this group from both avail and cache maps
			bucketTimes := make(map[int64]bool)
			for key := range bucketAvailMap {
				if key.GroupName == groupName {
					bucketTimes[key.BucketStart] = true
				}
			}
			for key := range bucketCacheMap {
				if key.GroupName == groupName {
					bucketTimes[key.BucketStart] = true
				}
			}
			for bucketStart := range bucketTimes {
				var bAvailRate float64 = -1
				var bCacheRate float64 = -1

				bKey := bucketGroupKey{bucketStart, groupName}
				if ba := bucketAvailMap[bKey]; ba != nil && ba.TotalRequests > 0 {
					bAvailRate = float64(ba.SuccessRequests) / float64(ba.TotalRequests) * 100
				}
				if bc := bucketCacheMap[bKey]; bc != nil && bc.CacheDataPoints > 0 {
					var totalTokens int64
					if isCacheSeparate {
						totalTokens = bc.TotalPromptTokens + bc.TotalCacheTokens
					} else {
						totalTokens = bc.TotalPromptTokens
					}
					if totalTokens > 0 {
						bCacheRate = float64(bc.TotalCacheTokens) / float64(totalTokens) * 100
						if bCacheRate > 100 {
							bCacheRate = 100
						}
					}
				}

				historyRecords = append(historyRecords, model.MonitoringHistory{
					GroupName:        groupName,
					AvailabilityRate: bAvailRate,
					CacheHitRate:     bCacheRate,
					RecordedAt:       bucketStart,
				})
			}
		} else {
			// Normal cycle: insert a single history record for the last interval
			var intervalAvailRate float64 = -1
			var intervalCacheRate float64 = -1

			if ia := intervalAvailMap[groupName]; ia != nil && ia.TotalRequests > 0 {
				intervalAvailRate = float64(ia.SuccessRequests) / float64(ia.TotalRequests) * 100
			}
			if ic := intervalCacheMap[groupName]; ic != nil && ic.CacheDataPoints > 0 {
				var totalTokens int64
				if isCacheSeparate {
					totalTokens = ic.TotalPromptTokens + ic.TotalCacheTokens
				} else {
					totalTokens = ic.TotalPromptTokens
				}
				if totalTokens > 0 {
					intervalCacheRate = float64(ic.TotalCacheTokens) / float64(totalTokens) * 100
					if intervalCacheRate > 100 {
						intervalCacheRate = 100
					}
				}
			}

			// No requests in this interval — carry forward from historical average
			if intervalAvailRate < 0 || intervalCacheRate < 0 {
				if intervalAvailRate < 0 {
					intervalAvailRate = carryForwardFromHistory(groupName, "availability")
				}
				if intervalCacheRate < 0 {
					intervalCacheRate = carryForwardFromHistory(groupName, "cache")
				}
			}

			historyRecords = append(historyRecords, model.MonitoringHistory{
				GroupName:        groupName,
				AvailabilityRate: intervalAvailRate,
				CacheHitRate:     intervalCacheRate,
				RecordedAt:       now,
			})
		}
	}

	if err := model.BatchInsertMonitoringHistory(historyRecords); err != nil {
		common.SysError("group monitoring: failed to insert history: " + err.Error())
	}

	// Step 8: Cleanup old data
	cleanupTime := now - 7*24*3600
	if _, err := model.CleanupOldRequestStats(cleanupTime); err != nil {
		common.SysError("group monitoring: failed to cleanup old request stats: " + err.Error())
	}
	historyCleanupTime := now - 30*24*3600
	if _, err := model.CleanupOldMonitoringHistory(historyCleanupTime); err != nil {
		common.SysError("group monitoring: failed to cleanup old monitoring history: " + err.Error())
	}

	// Cleanup stale groups
	model.CleanupStaleMonitoringStats(monitoringGroups)
}
