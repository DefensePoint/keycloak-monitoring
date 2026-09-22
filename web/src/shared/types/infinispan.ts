// Infinispan cache statistics for a single cache
export interface CacheStats {
  cache_name: string;

  // Size and entries
  approximate_entries: number;
  approximate_entries_unique: number;
  evictions: number;

  // Hit/Miss ratios
  hits: number;
  misses: number;
  hit_ratio: number;

  // Latencies (in milliseconds)
  hit_time_ms: number;
  miss_time_ms: number;
  store_time_ms: number;
  remove_hits: number;
  remove_misses: number;

  // Computed metrics
  read_write_ratio: number;
}

// Infinispan cluster and cache metrics
export interface InfinispanMetrics {
  // Cluster state
  cluster_size: number;
  required_minimum_nodes: number;

  // Replication and RPC
  replication_count: number;
  replication_failures: number;
  average_replication_time_ms: number;

  // Cache statistics by cache name
  cache_stats: Record<string, CacheStats>;

  // Locks and transactions
  number_of_locks_held: number;

  // JVM metrics
  jvm_memory_used_bytes: number;
  jvm_memory_committed_bytes: number;
  jvm_memory_used_percent: number;
  gc_pause_seconds: number;
  process_cpu_usage: number;

  // Infinispan status
  is_healthy: boolean;
}
