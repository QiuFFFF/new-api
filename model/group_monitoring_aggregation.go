package model

import (
	"fmt"
	"strings"

	"github.com/QuantumNous/new-api/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// LogAggRow represents a single row from the log aggregation query
type LogAggRow struct {
	BucketStart       int64
	GroupName         string
	ChannelId         int
	ModelName         string
	TotalRequests     int
	SuccessRequests   int
	ErrorRequests     int
	TotalCacheTokens  int64
	TotalPromptTokens int64
	CacheDataPoints   int
	SumResponseTime   int64
}

// rawLogAggRow is the per-(bucket, channel, model) aggregate before group expansion.
type rawLogAggRow struct {
	BucketStart       int64
	ChannelId         int
	ModelName         string
	TotalRequests     int
	SuccessRequests   int
	ErrorRequests     int
	TotalCacheTokens  int64
	TotalPromptTokens int64
	CacheDataPoints   int
	SumResponseTime   int64
}

// channelGroupRow holds the channel id and its comma-separated group string.
type channelGroupRow struct {
	Id    int
	Group string `gorm:"column:group"`
}

// getCacheTokensExpr returns the SQL expression for extracting cache_tokens from the Other JSON field
// Uses common.LogSqlType because LOG_DB may differ from main DB
func getCacheTokensExpr() string {
	switch common.LogSqlType {
	case common.DatabaseTypePostgreSQL:
		return `COALESCE(CAST(other::json->>'cache_tokens' AS BIGINT), 0)`
	case common.DatabaseTypeMySQL:
		return `COALESCE(CAST(JSON_EXTRACT(other, '$.cache_tokens') AS SIGNED), 0)`
	default: // SQLite
		return `COALESCE(CAST(json_extract(other, '$.cache_tokens') AS INTEGER), 0)`
	}
}

// getCacheDataPointExpr returns SQL CASE expression to count rows that have cache data
func getCacheDataPointExpr() string {
	switch common.LogSqlType {
	case common.DatabaseTypePostgreSQL:
		return `SUM(CASE WHEN other IS NOT NULL AND other != '' AND (other::json->>'cache_tokens') IS NOT NULL THEN 1 ELSE 0 END)`
	case common.DatabaseTypeMySQL:
		return `SUM(CASE WHEN other IS NOT NULL AND other != '' AND JSON_EXTRACT(other, '$.cache_tokens') IS NOT NULL THEN 1 ELSE 0 END)`
	default: // SQLite
		return `SUM(CASE WHEN other IS NOT NULL AND other != '' AND json_extract(other, '$.cache_tokens') IS NOT NULL THEN 1 ELSE 0 END)`
	}
}

// getBucketStartExpr returns SQL expression for bucket alignment
func getBucketStartExpr(intervalSeconds int64) string {
	return fmt.Sprintf("created_at - (created_at %% %d)", intervalSeconds)
}

// AggregateLogsForMonitoring queries LOG_DB and aggregates logs into request stats rows.
// It groups by (bucket, channel_id, model_name) — deliberately NOT by logs.group —
// then expands each row into one LogAggRow per channel group so that downstream logic
// can aggregate by channel group (channels.group) rather than by user token group (logs.group).
func AggregateLogsForMonitoring(
	startTime int64,
	endTime int64,
	intervalSeconds int64,
	availExcludeModels []string,
	cacheExcludeModels []string,
	excludeKeywords []string,
) ([]LogAggRow, error) {
	bucketExpr := getBucketStartExpr(intervalSeconds)
	cacheTokensExpr := getCacheTokensExpr()
	cacheDataPointExpr := getCacheDataPointExpr()

	// Step 1: Aggregate logs by (bucket, channel_id, model_name) — no logs.group filter.
	query := LOG_DB.Model(&Log{}).
		Select(fmt.Sprintf(
			`%s AS bucket_start,
			channel_id,
			model_name,
			COUNT(*) AS total_requests,
			SUM(CASE WHEN type = 2 THEN 1 ELSE 0 END) AS success_requests,
			SUM(CASE WHEN type = 5 THEN 1 ELSE 0 END) AS error_requests,
			SUM(%s) AS total_cache_tokens,
			SUM(prompt_tokens) AS total_prompt_tokens,
			%s AS cache_data_points,
			SUM(use_time) AS sum_response_time`,
			bucketExpr,
			cacheTokensExpr,
			cacheDataPointExpr,
		)).
		Where("created_at >= ? AND created_at < ?", startTime, endTime).
		Where("type IN (2, 5)")

	for _, keyword := range excludeKeywords {
		keyword = strings.TrimSpace(keyword)
		if keyword != "" {
			query = query.Where("content NOT LIKE ?", "%"+keyword+"%")
		}
	}

	query = query.Group(fmt.Sprintf("%s, channel_id, model_name", bucketExpr))

	var rawRows []rawLogAggRow
	if err := query.Find(&rawRows).Error; err != nil {
		return nil, err
	}
	if len(rawRows) == 0 {
		return nil, nil
	}

	// Step 2: Load channel_id → groups mapping from the main DB.
	channelIdSet := make(map[int]struct{}, len(rawRows))
	for _, r := range rawRows {
		channelIdSet[r.ChannelId] = struct{}{}
	}
	channelIds := make([]int, 0, len(channelIdSet))
	for id := range channelIdSet {
		channelIds = append(channelIds, id)
	}

	var channelRows []channelGroupRow
	if err := DB.Model(&Channel{}).
		Select("id, " + commonGroupCol).
		Where("id IN ?", channelIds).
		Find(&channelRows).Error; err != nil {
		return nil, err
	}

	// Build channel_id → []groupName map (channels may belong to multiple comma-separated groups).
	channelGroups := make(map[int][]string, len(channelRows))
	for _, cr := range channelRows {
		var groups []string
		for _, g := range strings.Split(cr.Group, ",") {
			g = strings.TrimSpace(g)
			if g != "" {
				groups = append(groups, g)
			}
		}
		if len(groups) > 0 {
			channelGroups[cr.Id] = groups
		}
	}

	// Step 3: Expand each raw row into one LogAggRow per channel group.
	var result []LogAggRow
	for _, r := range rawRows {
		groups, ok := channelGroups[r.ChannelId]
		if !ok {
			continue
		}
		for _, g := range groups {
			result = append(result, LogAggRow{
				BucketStart:       r.BucketStart,
				GroupName:         g,
				ChannelId:         r.ChannelId,
				ModelName:         r.ModelName,
				TotalRequests:     r.TotalRequests,
				SuccessRequests:   r.SuccessRequests,
				ErrorRequests:     r.ErrorRequests,
				TotalCacheTokens:  r.TotalCacheTokens,
				TotalPromptTokens: r.TotalPromptTokens,
				CacheDataPoints:   r.CacheDataPoints,
				SumResponseTime:   r.SumResponseTime,
			})
		}
	}
	return result, nil
}

// UpsertRequestStats batch upserts request stats into the main DB
func UpsertRequestStats(stats []RequestStat) error {
	if len(stats) == 0 {
		return nil
	}

	// Process in batches of 100
	batchSize := 100
	for i := 0; i < len(stats); i += batchSize {
		end := i + batchSize
		if end > len(stats) {
			end = len(stats)
		}
		batch := stats[i:end]

		err := DB.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "bucket_start"},
				{Name: "group_name"},
				{Name: "channel_id"},
				{Name: "model_name"},
			},
			DoUpdates: clause.AssignmentColumns([]string{
				"total_requests", "success_requests", "error_requests",
				"total_cache_tokens", "total_prompt_tokens", "cache_data_points",
				"sum_response_time",
			}),
		}).Create(&batch).Error

		if err != nil {
			// Fallback: use individual upsert if batch conflict fails
			// This handles cases where the unique index doesn't exist yet
			for _, stat := range batch {
				if saveErr := upsertSingleRequestStat(&stat); saveErr != nil {
					return saveErr
				}
			}
		}
	}
	return nil
}

func upsertSingleRequestStat(stat *RequestStat) error {
	var existing RequestStat
	err := DB.Where("bucket_start = ? AND group_name = ? AND channel_id = ? AND model_name = ?",
		stat.BucketStart, stat.GroupName, stat.ChannelId, stat.ModelName).First(&existing).Error
	if err != nil {
		// Not found, create
		return DB.Create(stat).Error
	}
	// Update
	return DB.Model(&existing).Updates(map[string]interface{}{
		"total_requests":     stat.TotalRequests,
		"success_requests":   stat.SuccessRequests,
		"error_requests":     stat.ErrorRequests,
		"total_cache_tokens": stat.TotalCacheTokens,
		"total_prompt_tokens": stat.TotalPromptTokens,
		"cache_data_points":  stat.CacheDataPoints,
		"sum_response_time":  stat.SumResponseTime,
	}).Error
}

// AvailabilityAggRow represents aggregated availability stats per group+channel
type AvailabilityAggRow struct {
	GroupName       string
	ChannelId       int
	TotalRequests   int
	SuccessRequests int
	ErrorRequests   int
	SumResponseTime int64
}

// CacheHitAggRow represents aggregated cache hit stats per group+channel
type CacheHitAggRow struct {
	GroupName         string
	ChannelId         int
	TotalCacheTokens  int64
	TotalPromptTokens int64
	CacheDataPoints   int
}

// GroupAvailabilityAggRow represents aggregated availability stats per group (no channel breakdown)
type GroupAvailabilityAggRow struct {
	GroupName       string
	TotalRequests   int
	SuccessRequests int
}

// AggregateAvailabilityByGroup aggregates request_stats for availability at the group level
func AggregateAvailabilityByGroup(bucketStart int64, excludeModels []string) ([]GroupAvailabilityAggRow, error) {
	query := DB.Model(&RequestStat{}).
		Select(`group_name,
			SUM(total_requests) as total_requests,
			SUM(success_requests) as success_requests`).
		Where("bucket_start >= ?", bucketStart).
		Group("group_name")

	if len(excludeModels) > 0 {
		query = query.Where("model_name NOT IN ?", excludeModels)
	}

	var rows []GroupAvailabilityAggRow
	err := query.Find(&rows).Error
	return rows, err
}

// GroupCacheHitAggRow represents aggregated cache hit stats per group (no channel breakdown)
type GroupCacheHitAggRow struct {
	GroupName         string
	TotalCacheTokens  int64
	TotalPromptTokens int64
	CacheDataPoints   int
}

// AggregateCacheHitByGroup aggregates request_stats for cache hit rate at the group level
func AggregateCacheHitByGroup(bucketStart int64, excludeModels []string) ([]GroupCacheHitAggRow, error) {
	query := DB.Model(&RequestStat{}).
		Select(`group_name,
			SUM(total_cache_tokens) as total_cache_tokens,
			SUM(total_prompt_tokens) as total_prompt_tokens,
			SUM(cache_data_points) as cache_data_points`).
		Where("bucket_start >= ?", bucketStart).
		Group("group_name")

	if len(excludeModels) > 0 {
		query = query.Where("model_name NOT IN ?", excludeModels)
	}

	var rows []GroupCacheHitAggRow
	err := query.Find(&rows).Error
	return rows, err
}

// GroupAvailabilityBucketRow represents availability stats per bucket_start + group
type GroupAvailabilityBucketRow struct {
	BucketStart     int64
	GroupName       string
	TotalRequests   int
	SuccessRequests int
}

// AggregateAvailabilityByGroupBucket aggregates request_stats for availability grouped by bucket_start and group_name
func AggregateAvailabilityByGroupBucket(startTime int64, excludeModels []string) ([]GroupAvailabilityBucketRow, error) {
	query := DB.Model(&RequestStat{}).
		Select(`bucket_start,
			group_name,
			SUM(total_requests) as total_requests,
			SUM(success_requests) as success_requests`).
		Where("bucket_start >= ?", startTime).
		Group("bucket_start, group_name")

	if len(excludeModels) > 0 {
		query = query.Where("model_name NOT IN ?", excludeModels)
	}

	var rows []GroupAvailabilityBucketRow
	err := query.Find(&rows).Error
	return rows, err
}

// GroupCacheHitBucketRow represents cache hit stats per bucket_start + group
type GroupCacheHitBucketRow struct {
	BucketStart       int64
	GroupName         string
	TotalCacheTokens  int64
	TotalPromptTokens int64
	CacheDataPoints   int
}

// AggregateCacheHitByGroupBucket aggregates request_stats for cache hit rate grouped by bucket_start and group_name
func AggregateCacheHitByGroupBucket(startTime int64, excludeModels []string) ([]GroupCacheHitBucketRow, error) {
	query := DB.Model(&RequestStat{}).
		Select(`bucket_start,
			group_name,
			SUM(total_cache_tokens) as total_cache_tokens,
			SUM(total_prompt_tokens) as total_prompt_tokens,
			SUM(cache_data_points) as cache_data_points`).
		Where("bucket_start >= ?", startTime).
		Group("bucket_start, group_name")

	if len(excludeModels) > 0 {
		query = query.Where("model_name NOT IN ?", excludeModels)
	}

	var rows []GroupCacheHitBucketRow
	err := query.Find(&rows).Error
	return rows, err
}

// AggregateAvailabilityByGroupChannel aggregates request_stats for availability calculation
func AggregateAvailabilityByGroupChannel(bucketStart int64, excludeModels []string) ([]AvailabilityAggRow, error) {
	query := DB.Model(&RequestStat{}).
		Select(`group_name, channel_id,
			SUM(total_requests) as total_requests,
			SUM(success_requests) as success_requests,
			SUM(error_requests) as error_requests,
			SUM(sum_response_time) as sum_response_time`).
		Where("bucket_start >= ?", bucketStart).
		Group("group_name, channel_id")

	if len(excludeModels) > 0 {
		query = query.Where("model_name NOT IN ?", excludeModels)
	}

	var rows []AvailabilityAggRow
	err := query.Find(&rows).Error
	return rows, err
}

// AggregateCacheHitByGroupChannel aggregates request_stats for cache hit rate calculation
func AggregateCacheHitByGroupChannel(bucketStart int64, excludeModels []string) ([]CacheHitAggRow, error) {
	query := DB.Model(&RequestStat{}).
		Select(`group_name, channel_id,
			SUM(total_cache_tokens) as total_cache_tokens,
			SUM(total_prompt_tokens) as total_prompt_tokens,
			SUM(cache_data_points) as cache_data_points`).
		Where("bucket_start >= ?", bucketStart).
		Group("group_name, channel_id")

	if len(excludeModels) > 0 {
		query = query.Where("model_name NOT IN ?", excludeModels)
	}

	var rows []CacheHitAggRow
	err := query.Find(&rows).Error
	return rows, err
}

// groupContainsCondition returns a SQL condition that matches a group name
// within a comma-separated list stored in the group column (e.g. "g1,g2,g3").
func groupContainsCondition() string {
	if common.UsingMySQL {
		return `CONCAT(',', ` + commonGroupCol + `, ',') LIKE ?`
	}
	return `(',' || ` + commonGroupCol + ` || ',') LIKE ?`
}

// groupContainsArg returns the LIKE argument for groupContainsCondition.
func groupContainsArg(groupName string) string {
	return "%," + groupName + ",%"
}

// GetChannelsByGroup retrieves all enabled channels that belong to a specific group
func GetChannelsByGroup(groupName string) ([]*Channel, error) {
	var channels []*Channel
	err := DB.Where(groupContainsCondition()+" AND status = 1", groupContainsArg(groupName)).Find(&channels).Error
	return channels, err
}

// GetAllChannelsByGroup retrieves all channels (regardless of status) that belong to a specific group
func GetAllChannelsByGroup(groupName string) ([]*Channel, error) {
	var channels []*Channel
	err := DB.Where(groupContainsCondition(), groupContainsArg(groupName)).Find(&channels).Error
	return channels, err
}

// GetAllGroupNames returns all distinct group names from enabled channels
func GetAllGroupNames() ([]string, error) {
	var groups []string
	err := DB.Model(&Channel{}).Where("status = 1").Distinct(commonGroupCol).Pluck(commonGroupCol, &groups).Error
	return groups, err
}

// UpsertChannelMonitoringStat upserts a channel monitoring stat record
func UpsertChannelMonitoringStat(stat *ChannelMonitoringStat) error {
	var existing ChannelMonitoringStat
	err := DB.Where("group_name = ? AND channel_id = ?", stat.GroupName, stat.ChannelId).First(&existing).Error
	if err != nil {
		return DB.Create(stat).Error
	}
	return DB.Model(&existing).Updates(map[string]interface{}{
		"availability_rate":  stat.AvailabilityRate,
		"cache_hit_rate":     stat.CacheHitRate,
		"last_response_time": stat.LastResponseTime,
		"last_frt":           stat.LastFRT,
		"last_test_time":     stat.LastTestTime,
		"last_test_model":    stat.LastTestModel,
		"is_online":          stat.IsOnline,
		"updated_at":         stat.UpdatedAt,
	}).Error
}

// UpsertGroupMonitoringStat upserts a group monitoring stat record
func UpsertGroupMonitoringStat(stat *GroupMonitoringStat) error {
	var existing GroupMonitoringStat
	err := DB.Where("group_name = ?", stat.GroupName).First(&existing).Error
	if err != nil {
		return DB.Create(stat).Error
	}
	return DB.Model(&existing).Updates(map[string]interface{}{
		"availability_rate": stat.AvailabilityRate,
		"cache_hit_rate":    stat.CacheHitRate,
		"avg_response_time": stat.AvgResponseTime,
		"avg_frt":           stat.AvgFRT,
		"online_channels":   stat.OnlineChannels,
		"total_channels":    stat.TotalChannels,
		"group_ratio":       stat.GroupRatio,
		"last_test_model":   stat.LastTestModel,
		"updated_at":        stat.UpdatedAt,
	}).Error
}

// InsertMonitoringHistory inserts a new monitoring history record
func InsertMonitoringHistory(history *MonitoringHistory) error {
	return DB.Create(history).Error
}

// IsRequestStatsEmpty returns true if the request_stats table has no rows.
// Used at startup to decide whether to run a full-refresh backfill.
func IsRequestStatsEmpty() bool {
	var count int64
	DB.Model(&RequestStat{}).Count(&count)
	return count == 0
}

// GetAllGroupMonitoringStats returns all group monitoring stats
func GetAllGroupMonitoringStats() ([]GroupMonitoringStat, error) {
	var stats []GroupMonitoringStat
	err := DB.Find(&stats).Error
	return stats, err
}

// GetGroupMonitoringStatByName returns a single group monitoring stat
func GetGroupMonitoringStatByName(groupName string) (*GroupMonitoringStat, error) {
	var stat GroupMonitoringStat
	err := DB.Where("group_name = ?", groupName).First(&stat).Error
	if err != nil {
		return nil, err
	}
	return &stat, nil
}

// GetChannelMonitoringStatsByGroup returns all channel monitoring stats for a group
func GetChannelMonitoringStatsByGroup(groupName string) ([]ChannelMonitoringStat, error) {
	var stats []ChannelMonitoringStat
	err := DB.Where("group_name = ?", groupName).Find(&stats).Error
	return stats, err
}

// GetMonitoringHistory returns monitoring history for a group within a time range
func GetMonitoringHistory(groupName string, startTime int64, endTime int64) ([]MonitoringHistory, error) {
	var history []MonitoringHistory
	err := DB.Where("group_name = ? AND recorded_at >= ? AND recorded_at <= ?", groupName, startTime, endTime).
		Order("recorded_at ASC").Find(&history).Error
	return history, err
}

// GetLastMonitoringHistoryBefore returns the most recent history record before the given timestamp.
// Used as a seed value for carry-forward when the period window has no data.
func GetLastMonitoringHistoryBefore(groupName string, before int64) (*MonitoringHistory, error) {
	var h MonitoringHistory
	err := DB.Where("group_name = ? AND recorded_at < ?", groupName, before).
		Order("recorded_at DESC").First(&h).Error
	if err != nil {
		return nil, err
	}
	return &h, nil
}

// GetRecentAvailabilityAvg returns the average availability_rate from the most recent `limit`
// valid records (availability_rate >= 0) for a group. Returns -1 if no data exists.
func GetRecentAvailabilityAvg(groupName string, limit int) float64 {
	// Use subquery: AVG over the top-N rows ordered by recorded_at DESC
	sub := DB.Model(&MonitoringHistory{}).
		Select("availability_rate").
		Where("group_name = ? AND availability_rate >= 0", groupName).
		Order("recorded_at DESC").
		Limit(limit)

	var avg *float64
	err := DB.Table("(?) AS sub", sub).Select("AVG(sub.availability_rate)").Scan(&avg).Error
	if err != nil || avg == nil {
		return -1
	}
	return *avg
}

// GetRecentCacheHitAvg returns the average cache_hit_rate from the most recent `limit`
// valid records (cache_hit_rate >= 0) for a group. Returns -1 if no data exists.
func GetRecentCacheHitAvg(groupName string, limit int) float64 {
	sub := DB.Model(&MonitoringHistory{}).
		Select("cache_hit_rate").
		Where("group_name = ? AND cache_hit_rate >= 0", groupName).
		Order("recorded_at DESC").
		Limit(limit)

	var avg *float64
	err := DB.Table("(?) AS sub", sub).Select("AVG(sub.cache_hit_rate)").Scan(&avg).Error
	if err != nil || avg == nil {
		return -1
	}
	return *avg
}

// CleanupOldRequestStats deletes request_stats older than the given timestamp
func CleanupOldRequestStats(before int64) (int64, error) {
	result := DB.Where("bucket_start < ?", before).Delete(&RequestStat{})
	return result.RowsAffected, result.Error
}

// CleanupOldMonitoringHistory deletes monitoring_history older than the given timestamp
func CleanupOldMonitoringHistory(before int64) (int64, error) {
	result := DB.Where("recorded_at < ?", before).Delete(&MonitoringHistory{})
	return result.RowsAffected, result.Error
}

// DeleteRequestStatsByGroup deletes all request_stats for a specific group
func DeleteRequestStatsByGroup(groupName string) (int64, error) {
	result := DB.Where("group_name = ?", groupName).Delete(&RequestStat{})
	return result.RowsAffected, result.Error
}

// DeleteChannelMonitoringStatsByGroup deletes all channel monitoring stats for a group
func DeleteChannelMonitoringStatsByGroup(groupName string) (int64, error) {
	result := DB.Where("group_name = ?", groupName).Delete(&ChannelMonitoringStat{})
	return result.RowsAffected, result.Error
}

// DeleteOrphanChannelMonitoringStats deletes channel monitoring stats for channels not in the active set
func DeleteOrphanChannelMonitoringStats(groupName string, activeChannelIds []int) {
	if len(activeChannelIds) == 0 {
		return
	}
	DB.Where("group_name = ? AND channel_id NOT IN ?", groupName, activeChannelIds).Delete(&ChannelMonitoringStat{})
}

// DeleteGroupMonitoringStatByGroup deletes the group monitoring stat record
func DeleteGroupMonitoringStatByGroup(groupName string) (int64, error) {
	result := DB.Where("group_name = ?", groupName).Delete(&GroupMonitoringStat{})
	return result.RowsAffected, result.Error
}

// DeleteMonitoringHistoryByGroup deletes all monitoring history for a group
func DeleteMonitoringHistoryByGroup(groupName string) (int64, error) {
	result := DB.Where("group_name = ?", groupName).Delete(&MonitoringHistory{})
	return result.RowsAffected, result.Error
}

// DeleteRecentRequestStats deletes request_stats within the last N seconds (for refresh)
func DeleteRecentRequestStats(since int64) (int64, error) {
	result := DB.Where("bucket_start >= ?", since).Delete(&RequestStat{})
	return result.RowsAffected, result.Error
}

// DeleteAllMonitoringData clears all monitoring tables for a full re-aggregation
func DeleteAllMonitoringData() {
	DB.Where("1 = 1").Delete(&RequestStat{})
	DB.Where("1 = 1").Delete(&ChannelMonitoringStat{})
	DB.Where("1 = 1").Delete(&GroupMonitoringStat{})
	DB.Where("1 = 1").Delete(&MonitoringHistory{})
}

// GetGroupMonitoringStatsByNames returns group monitoring stats for specific group names
func GetGroupMonitoringStatsByNames(names []string) ([]GroupMonitoringStat, error) {
	var stats []GroupMonitoringStat
	if len(names) == 0 {
		return stats, nil
	}
	err := DB.Where("group_name IN ?", names).Find(&stats).Error
	return stats, err
}

// CleanupStaleMonitoringStats removes stats for groups that no longer exist in MonitoringGroups list
func CleanupStaleMonitoringStats(activeGroups []string) error {
	if len(activeGroups) == 0 {
		return nil
	}
	// Delete channel stats for non-active groups
	DB.Where("group_name NOT IN ?", activeGroups).Delete(&ChannelMonitoringStat{})
	// Delete group stats for non-active groups
	DB.Where("group_name NOT IN ?", activeGroups).Delete(&GroupMonitoringStat{})
	return nil
}

// GetAllMonitoringHistoryAfter returns all monitoring history after a given time
func GetAllMonitoringHistoryAfter(startTime int64) ([]MonitoringHistory, error) {
	var history []MonitoringHistory
	err := DB.Where("recorded_at >= ?", startTime).Order("recorded_at ASC").Find(&history).Error
	return history, err
}

// CheckMonitoringTablesExist checks if monitoring tables exist
func CheckMonitoringTablesExist() bool {
	return DB.Migrator().HasTable(&RequestStat{}) &&
		DB.Migrator().HasTable(&GroupMonitoringStat{})
}

// GetGroupMonitoringStatsForPublic returns limited group stats for public/user view
func GetGroupMonitoringStatsForPublic(names []string) ([]GroupMonitoringStat, error) {
	var stats []GroupMonitoringStat
	if len(names) == 0 {
		return stats, nil
	}
	err := DB.Where("group_name IN ?", names).Find(&stats).Error
	return stats, err
}

// BatchInsertMonitoringHistory batch inserts monitoring history records
func BatchInsertMonitoringHistory(records []MonitoringHistory) error {
	if len(records) == 0 {
		return nil
	}
	return DB.Create(&records).Error
}

// ChannelTestInfo holds channel test information for monitoring
type ChannelTestInfo struct {
	ChannelId    int
	Group        string
	TestTime     int64
	ResponseTime int
	TestModel    string
	Status       int
}

// GetChannelTestInfoByGroups returns channel test info for specified groups
// Uses LIKE matching to support comma-separated group values in channels.
func GetChannelTestInfoByGroups(groups []string) ([]ChannelTestInfo, error) {
	if len(groups) == 0 {
		return nil, nil
	}

	// Build OR conditions for each group using LIKE matching
	db := DB.Model(&Channel{}).Where("status = 1")
	condition := groupContainsCondition()
	orClauses := DB.Where(condition, groupContainsArg(groups[0]))
	for _, g := range groups[1:] {
		orClauses = orClauses.Or(condition, groupContainsArg(g))
	}
	db = db.Where(orClauses)

	var channels []*Channel
	err := db.Find(&channels).Error
	if err != nil {
		return nil, err
	}
	results := make([]ChannelTestInfo, len(channels))
	for i, ch := range channels {
		testModel := ""
		if ch.TestModel != nil {
			testModel = *ch.TestModel
		}
		results[i] = ChannelTestInfo{
			ChannelId:    ch.Id,
			Group:        ch.Group,
			TestTime:     ch.TestTime,
			ResponseTime: ch.ResponseTime,
			TestModel:    testModel,
			Status:       ch.Status,
		}
	}
	return results, nil
}

// countChannelsByGroup counts enabled channels per group
func CountChannelsByGroup(groupName string) (int64, error) {
	var count int64
	err := DB.Model(&Channel{}).Where(groupContainsCondition()+" AND status = 1", groupContainsArg(groupName)).Count(&count).Error
	return count, err
}

// GetDistinctMonitoredGroups returns distinct group names that have monitoring stats
func GetDistinctMonitoredGroups() ([]string, error) {
	var groups []string
	err := DB.Model(&GroupMonitoringStat{}).Distinct("group_name").Pluck("group_name", &groups).Error
	return groups, err
}

// frtLogRow holds a single row from the FRT query
type frtLogRow struct {
	ChannelId int
	Other     string
}

// GetLatestFRTForChannels returns the most recent frt value (from Log.Other JSON) for each channel
// within the given time range. Group filtering is intentionally omitted — online status is based
// on actual channel traffic regardless of which user group made the request.
func GetLatestFRTForChannels(startTime int64, _ string, channelIds []int) (map[int]int, error) {
	if len(channelIds) == 0 {
		return nil, nil
	}

	var rows []frtLogRow
	err := LOG_DB.Model(&Log{}).
		Select("channel_id, other").
		Where("created_at >= ?", startTime).
		Where("channel_id IN ?", channelIds).
		Where("other LIKE ?", `%"frt"%`).
		Order("created_at DESC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}

	result := make(map[int]int)
	seen := make(map[int]bool)
	for _, row := range rows {
		if seen[row.ChannelId] {
			continue
		}
		// Parse Other JSON to extract frt
		var otherMap map[string]interface{}
		if err := common.Unmarshal([]byte(row.Other), &otherMap); err != nil {
			continue
		}
		if frtVal, ok := otherMap["frt"]; ok {
			var frt int
			switch v := frtVal.(type) {
			case float64:
				frt = int(v)
			case int:
				frt = v
			default:
				continue
			}
			if frt > 0 {
				result[row.ChannelId] = frt
				seen[row.ChannelId] = true
			}
		}
	}
	return result, nil
}

// logStatusRow holds channel_id and type from the latest log
type logStatusRow struct {
	ChannelId int
	Type      int
}

// GetLatestLogStatusForChannels returns a map[channelId]bool indicating online status
// based on the most recent log entry for each channel.
// It first looks within the period (startTime~now). For channels with no logs in the
// period, it falls back to the most recent log before startTime so that idle channels
// retain their last-known status instead of being marked offline.
// type=2 (success) → true (online), type=5 (error) → false (offline), no log at all → absent (offline).
// Group filtering is intentionally omitted — channel availability is determined by actual
// traffic to the channel regardless of which user group made the request.
func GetLatestLogStatusForChannels(
	startTime int64,
	_ string,
	channelIds []int,
	excludeModels []string,
	excludeKeywords []string,
) (map[int]bool, error) {
	if len(channelIds) == 0 {
		return nil, nil
	}

	buildQuery := func(q *gorm.DB) *gorm.DB {
		if len(excludeModels) > 0 {
			q = q.Where("model_name NOT IN ?", excludeModels)
		}
		for _, keyword := range excludeKeywords {
			keyword = strings.TrimSpace(keyword)
			if keyword != "" {
				q = q.Where("NOT (type = 5 AND content LIKE ?)", "%"+keyword+"%")
			}
		}
		return q
	}

	// 1) Look within the period
	query := LOG_DB.Model(&Log{}).
		Select("channel_id, type").
		Where("created_at >= ?", startTime).
		Where("channel_id IN ?", channelIds).
		Where("type IN (2, 5)")
	query = buildQuery(query)

	var rows []logStatusRow
	err := query.Order("created_at DESC").Find(&rows).Error
	if err != nil {
		return nil, err
	}

	result := make(map[int]bool)
	seen := make(map[int]bool)
	for _, row := range rows {
		if seen[row.ChannelId] {
			continue
		}
		seen[row.ChannelId] = true
		result[row.ChannelId] = (row.Type == 2)
	}

	// 2) For channels without logs in the period, fall back to latest log before startTime
	var missingIds []int
	for _, id := range channelIds {
		if !seen[id] {
			missingIds = append(missingIds, id)
		}
	}
	if len(missingIds) > 0 {
		// Limit fallback lookback to 30 days to avoid unbounded full-history scans.
		const fallbackLookbackSeconds = int64(30 * 24 * 3600)
		fallbackQuery := LOG_DB.Model(&Log{}).
			Select("channel_id, type").
			Where("created_at >= ? AND created_at < ?", startTime-fallbackLookbackSeconds, startTime).
			Where("channel_id IN ?", missingIds).
			Where("type IN (2, 5)")
		fallbackQuery = buildQuery(fallbackQuery)

		var fallbackRows []logStatusRow
		err = fallbackQuery.Order("created_at DESC").Find(&fallbackRows).Error
		if err != nil {
			return result, nil // non-fatal, return what we have
		}
		for _, row := range fallbackRows {
			if seen[row.ChannelId] {
				continue
			}
			seen[row.ChannelId] = true
			result[row.ChannelId] = (row.Type == 2)
		}
	}

	return result, nil
}

// logModelRow holds channel_id and model_name from the latest log
type logModelRow struct {
	ChannelId int
	ModelName string
}

// GetLatestModelForChannels returns the most recent model_name for each channel
// by looking at the latest log entry (type=2 or type=5) with a non-empty model_name.
// This is used as a fallback when Channel.TestModel is not configured.
// Group filtering is intentionally omitted — model detection is based on actual channel usage.
func GetLatestModelForChannels(_ string, channelIds []int) (map[int]string, error) {
	if len(channelIds) == 0 {
		return nil, nil
	}

	var rows []logModelRow
	err := LOG_DB.Model(&Log{}).
		Select("channel_id, model_name").
		Where("channel_id IN ?", channelIds).
		Where("type IN (2, 5)").
		Where("model_name != ''").
		Order("created_at DESC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}

	result := make(map[int]string)
	seen := make(map[int]bool)
	for _, row := range rows {
		if seen[row.ChannelId] {
			continue
		}
		seen[row.ChannelId] = true
		result[row.ChannelId] = row.ModelName
	}
	return result, nil
}

// transactional helper to delete all monitoring data for a group
func DeleteAllMonitoringDataForGroup(groupName string) (totalDeleted int64, err error) {
	tx := DB.Begin()
	if tx.Error != nil {
		return 0, tx.Error
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()

	result := tx.Where("group_name = ?", groupName).Delete(&RequestStat{})
	if result.Error != nil {
		return 0, result.Error
	}
	totalDeleted += result.RowsAffected

	result = tx.Where("group_name = ?", groupName).Delete(&ChannelMonitoringStat{})
	if result.Error != nil {
		return 0, result.Error
	}
	totalDeleted += result.RowsAffected

	result = tx.Where("group_name = ?", groupName).Delete(&GroupMonitoringStat{})
	if result.Error != nil {
		return 0, result.Error
	}
	totalDeleted += result.RowsAffected

	result = tx.Where("group_name = ?", groupName).Delete(&MonitoringHistory{})
	if result.Error != nil {
		return 0, result.Error
	}
	totalDeleted += result.RowsAffected

	err = tx.Commit().Error
	return totalDeleted, err
}
