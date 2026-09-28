package keycloakadmin

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// ============================================================================
// INFINISPAN METRICS MODELS
// ============================================================================

// InfinispanMetrics represents parsed Infinispan metrics from Prometheus format
type InfinispanMetrics struct {
	// Cluster state
	ClusterSize                  float64 `json:"cluster_size"`
	RequiredMinimumNumberOfNodes float64 `json:"required_minimum_nodes"`

	// Replication and RPC
	ReplicationCount       float64 `json:"replication_count"`
	ReplicationFailures    float64 `json:"replication_failures"`
	AverageReplicationTime float64 `json:"average_replication_time_ms"`

	// Cache statistics by cache name
	CacheStats map[string]*CacheStats `json:"cache_stats"`

	// Locks and transactions
	NumberOfLocksHeld float64 `json:"number_of_locks_held"`

	// JVM metrics
	JVMMemoryUsed        float64 `json:"jvm_memory_used_bytes"`
	JVMMemoryCommitted   float64 `json:"jvm_memory_committed_bytes"`
	JVMMemoryUsedPercent float64 `json:"jvm_memory_used_percent"`
	GCPauseSeconds       float64 `json:"gc_pause_seconds"`
	ProcessCPUUsage      float64 `json:"process_cpu_usage"`

	// Infinispan status
	IsHealthy bool `json:"is_healthy"`
}

// CacheStats represents statistics for a single cache
type CacheStats struct {
	CacheName string `json:"cache_name"`

	// Size and entries
	ApproximateEntries       float64 `json:"approximate_entries"`
	ApproximateEntriesUnique float64 `json:"approximate_entries_unique"`
	Evictions                float64 `json:"evictions"`

	// Hit/Miss ratios
	Hits     float64 `json:"hits"`
	Misses   float64 `json:"misses"`
	HitRatio float64 `json:"hit_ratio"`

	// Latencies (in seconds from Prometheus, converted to ms)
	HitTimeSeconds   float64 `json:"hit_time_ms"`
	MissTimeSeconds  float64 `json:"miss_time_ms"`
	StoreTimeSeconds float64 `json:"store_time_ms"`
	RemoveHits       float64 `json:"remove_hits"`
	RemoveMisses     float64 `json:"remove_misses"`

	// Computed metrics
	ReadWriteRatio float64 `json:"read_write_ratio"`
}

// SimpleInfinispanMetrics - simplified version focused on cluster status
type SimpleInfinispanMetrics struct {
	// Cluster Status (based on JGroups connections)
	ClusterStatus      string `json:"cluster_status"`      // "UP", "DEGRADED", "DOWN"
	ClusterConnections int    `json:"cluster_connections"` // Number of JGroups connections

	// JVM Basics
	MemoryUsedMB       float64 `json:"memory_used_mb"`
	MemoryCommittedMB  float64 `json:"memory_committed_mb"`
	MemoryUsagePercent float64 `json:"memory_usage_percent"`

	// CPU
	CPUUsagePercent     float64 `json:"cpu_usage_percent"`    // Process CPU load
	SystemLoadAverage   float64 `json:"system_load_average"`  // System load (1 min)
	AvailableProcessors int     `json:"available_processors"` // Number of CPUs
	ThreadCount         int     `json:"thread_count"`         // Total threads

	// Metadata
	Timestamp time.Time `json:"timestamp"`
}

// ============================================================================
// PROMETHEUS METRICS PARSER
// ============================================================================

// GetInfinispanMetrics fetches and parses Infinispan metrics from Keycloak /metrics endpoint
func (c *Client) GetInfinispanMetrics(ctx context.Context) (*InfinispanMetrics, error) {
	// Build metrics URL with custom port if specified
	metricsURL := c.buildMetricsURL()

	c.logger.Debug("Fetching Infinispan metrics", "url", metricsURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metricsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create metrics request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch metrics: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("metrics endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	metrics, err := parsePrometheusMetrics(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse metrics: %w", err)
	}

	return metrics, nil
}

// parsePrometheusMetrics parses Prometheus text format metrics
func parsePrometheusMetrics(reader io.Reader) (*InfinispanMetrics, error) {
	scanner := bufio.NewScanner(reader)

	metrics := &InfinispanMetrics{
		CacheStats: make(map[string]*CacheStats),
		IsHealthy:  true,
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse metric line: metric_name{labels} value
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		metricName, labelsStr := parseMetricNameAndLabels(parts[0])
		value, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			continue // Skip invalid values
		}

		labels := parseLabels(labelsStr)

		// Parse cluster metrics
		switch {
		case metricName == "vendor_cluster_size":
			metrics.ClusterSize = value
		case metricName == "vendor_statistics_required_minimum_number_of_nodes":
			metrics.RequiredMinimumNumberOfNodes = value
		case metricName == "vendor_rpc_manager_replication_count":
			metrics.ReplicationCount = value
		case metricName == "vendor_rpc_manager_replication_failures":
			metrics.ReplicationFailures = value
		case metricName == "vendor_rpc_manager_average_replication_time":
			metrics.AverageReplicationTime = value
		case metricName == "vendor_lock_manager_number_of_locks_held":
			metrics.NumberOfLocksHeld = value
		case metricName == "jvm_memory_used_bytes":
			metrics.JVMMemoryUsed = value
		case metricName == "jvm_memory_committed_bytes":
			metrics.JVMMemoryCommitted = value
		case metricName == "jvm_gc_pause_seconds_sum":
			metrics.GCPauseSeconds = value
		case metricName == "process_cpu_usage":
			metrics.ProcessCPUUsage = value

		// Parse cache-specific metrics
		case strings.HasPrefix(metricName, "vendor_statistics_") || strings.HasPrefix(metricName, "vendor_cache_"):
			cacheName := labels["cache"]
			if cacheName == "" {
				continue
			}

			if metrics.CacheStats[cacheName] == nil {
				metrics.CacheStats[cacheName] = &CacheStats{
					CacheName: cacheName,
				}
			}

			cache := metrics.CacheStats[cacheName]

			switch metricName {
			case "vendor_statistics_approximate_entries":
				cache.ApproximateEntries = value
			case "vendor_statistics_approximate_entries_unique":
				cache.ApproximateEntriesUnique = value
			case "vendor_statistics_evictions":
				cache.Evictions = value
			case "vendor_statistics_hits":
				cache.Hits = value
			case "vendor_statistics_misses":
				cache.Misses = value
			case "vendor_statistics_hit_times_seconds_sum", "vendor_statistics_hit_times_seconds":
				cache.HitTimeSeconds = value * 1000 // Convert to ms
			case "vendor_statistics_miss_times_seconds_sum", "vendor_statistics_miss_times_seconds":
				cache.MissTimeSeconds = value * 1000 // Convert to ms
			case "vendor_statistics_store_times_seconds_sum", "vendor_statistics_store_times_seconds":
				cache.StoreTimeSeconds = value * 1000 // Convert to ms
			case "vendor_statistics_remove_hits":
				cache.RemoveHits = value
			case "vendor_statistics_remove_misses":
				cache.RemoveMisses = value
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading metrics: %w", err)
	}

	// Calculate derived metrics
	calculateDerivedMetrics(metrics)

	return metrics, nil
}

// parseMetricNameAndLabels splits "metric_name{labels}" into parts
func parseMetricNameAndLabels(metricStr string) (name string, labels string) {
	idx := strings.Index(metricStr, "{")
	if idx == -1 {
		return metricStr, ""
	}
	return metricStr[:idx], metricStr[idx+1 : len(metricStr)-1]
}

// parseLabels parses label string "key1=\"value1\",key2=\"value2\"" into map
func parseLabels(labelsStr string) map[string]string {
	labels := make(map[string]string)
	if labelsStr == "" {
		return labels
	}

	pairs := strings.Split(labelsStr, ",")
	for _, pair := range pairs {
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) == 2 {
			key := strings.TrimSpace(kv[0])
			value := strings.Trim(strings.TrimSpace(kv[1]), "\"")
			labels[key] = value
		}
	}

	return labels
}

// calculateDerivedMetrics calculates hit ratios and other derived metrics
func calculateDerivedMetrics(metrics *InfinispanMetrics) {
	// Calculate JVM memory percentage
	if metrics.JVMMemoryCommitted > 0 {
		metrics.JVMMemoryUsedPercent = (metrics.JVMMemoryUsed / metrics.JVMMemoryCommitted) * 100
	}

	// Calculate cache-level derived metrics
	for _, cache := range metrics.CacheStats {
		// Hit ratio
		totalReads := cache.Hits + cache.Misses
		if totalReads > 0 {
			cache.HitRatio = (cache.Hits / totalReads) * 100
		}

		// Read-write ratio
		totalOps := cache.Hits + cache.Misses + cache.RemoveHits + cache.RemoveMisses
		readOps := cache.Hits + cache.Misses
		if totalOps > 0 {
			cache.ReadWriteRatio = (readOps / totalOps) * 100
		}
	}

	// Check health
	if metrics.ClusterSize > 0 && metrics.RequiredMinimumNumberOfNodes > 0 {
		metrics.IsHealthy = metrics.ClusterSize >= metrics.RequiredMinimumNumberOfNodes
	}

	// High replication failures is a red flag
	if metrics.ReplicationFailures > 100 {
		metrics.IsHealthy = false
	}
}

// GetSimpleInfinispanMetrics fetches and parses ONLY available metrics
func (c *Client) GetSimpleInfinispanMetrics(ctx context.Context) (*SimpleInfinispanMetrics, error) {
	// Build metrics URL with custom port if specified
	metricsURL := c.buildMetricsURL()

	// Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, metricsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create metrics request: %w", err)
	}

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch metrics: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("metrics endpoint returned status %d", resp.StatusCode)
	}

	// Parse metrics
	metrics := &SimpleInfinispanMetrics{
		Timestamp: time.Now(),
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse metric line
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}

		metricName := parts[0]
		valueStr := parts[len(parts)-1]

		// Extract value
		value, err := strconv.ParseFloat(valueStr, 64)
		if err != nil {
			continue
		}

		// Parse specific metrics we care about
		switch {
		// JGroups cluster connections
		case strings.Contains(metricName, "vendor_jgroups") && strings.Contains(metricName, "num_send_connections"):
			metrics.ClusterConnections = int(value)

		// JVM Memory
		case metricName == "base_memory_usedHeap_bytes":
			metrics.MemoryUsedMB = value / (1024 * 1024)
		case metricName == "base_memory_committedHeap_bytes":
			metrics.MemoryCommittedMB = value / (1024 * 1024)

		// CPU
		case metricName == "base_cpu_processCpuLoad":
			metrics.CPUUsagePercent = value * 100 // Convert to percentage
		case metricName == "base_cpu_systemLoadAverage":
			metrics.SystemLoadAverage = value
		case metricName == "base_cpu_availableProcessors":
			metrics.AvailableProcessors = int(value)

		// Threads
		case metricName == "base_thread_count":
			metrics.ThreadCount = int(value)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading metrics: %w", err)
	}

	// Calculate memory percentage
	if metrics.MemoryCommittedMB > 0 {
		metrics.MemoryUsagePercent = (metrics.MemoryUsedMB / metrics.MemoryCommittedMB) * 100
	}

	// Determine cluster status based on connections
	if metrics.ClusterConnections > 0 {
		metrics.ClusterStatus = "UP"
	} else {
		metrics.ClusterStatus = "UNKNOWN"
	}

	c.logger.Info("Simple Infinispan metrics collected",
		"status", metrics.ClusterStatus,
		"connections", metrics.ClusterConnections)

	return metrics, nil
}
