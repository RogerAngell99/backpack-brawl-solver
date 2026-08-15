export type CoordTuple = [number, number];

export interface Star {
  offset: CoordTuple;
  target_types: string[];
  target_items: string[];
  exclude_source_item?: boolean;
  rule_status?: "known" | "unknown";
  effect_text: string;
}

export interface ItemAlias {
  item_id: string;
  count: number;
}

export interface CatalogItem {
  id: string;
  name: string;
  types: string[];
  shape: CoordTuple[];
  stars: Star[];
  counts_as?: ItemAlias[];
  ability_text: string;
  source_url: string;
  image_url: string;
  image_path: string;
  needs_review: boolean;
}

export interface Recipe {
  result: string;
  anchor: string;
  ingredients: string[];
  source_url: string;
}

export interface Catalog {
  items: CatalogItem[];
  recipes: Recipe[];
}

export interface RuntimeStat {
  type: string;
  value: number;
}

export interface RuntimeLevelEffect {
  level: number;
  kind: string;
  value: number | null;
  stat_target: string | null;
  modifier_type: string | null;
}

export interface RuntimeItemMetadata {
  catalog_id: string;
  client_id: string;
  name: string;
  asset_id: string;
  rarity: number | null;
  layer: number | null;
  stats: RuntimeStat[];
  levels: {
    max_level: number | null;
    effects: RuntimeLevelEffect[];
  };
  ability_text?: string;
  ability_text_pt_br?: string;
  star_status?: {
    available: boolean;
    reason?: string;
  };
}

export interface RuntimeMetadata {
  schema_version: number;
  game_version: string;
  source: string;
  items: RuntimeItemMetadata[];
}

export interface Scenario {
  name?: string;
  grid?: string[];
  items: Record<string, number>;
  top?: number;
  workers?: number;
  max_nodes?: number;
  no_skips?: boolean;
  stop_on_coverage_ceiling?: boolean;
  repair_search?: boolean;
  priorities?: string[];
  coverage_groups?: CoverageGroup[];
}

export interface CoverageGroup {
  name: string;
  sources: string[];
  targets?: string[];
}

export interface SolutionScore {
  crafts: number;
  stars: number;
  items: number;
  priority_counts?: number[];
}

export interface CoverageBucket {
  covered_sources: number;
  target_count: number;
}

export interface CoverageTarget {
  target_instance: string;
  target_item_id: string;
  covered_sources: string[] | null;
  covered_count: number;
}

export interface CoverageBreakdown {
  name?: string;
  sources: string[];
  target_item_ids?: string[];
  buckets: CoverageBucket[];
  targets: CoverageTarget[];
  summary: string;
}

export interface LooseStarPriority {
  source_item_id: string;
  target_count: number;
}

export interface SearchStats {
  nodes_explored: number;
  nodes_per_second?: number;
  backend?: string;
  server_elapsed_ms?: number;
  remote_workers?: number;
  max_nodes_applied?: number;
  max_nodes_capped?: boolean;
  limited: boolean;
  refined: boolean;
  coverage_sources?: string[];
  coverage_target_count?: number;
  coverage_ceiling?: CoverageBucket[];
  coverage_ceiling_reached?: boolean;
  coverage_bound_checks?: number;
  coverage_pruned_nodes?: number;
  exact_bound_checks?: number;
  exact_bound_pruned_nodes?: number;
  coverage_seed_nodes?: number;
  coverage_seed_candidates?: number;
  coverage_seed_best?: string;
  parallel_tasks?: number;
  parallel_workers_used?: number;
  refine_moves_checked?: number;
  refine_improvements?: number;
  refine_best_delta?: string;
  repair_nodes?: number;
  repair_iterations?: number;
  repair_improvements?: number;
  repair_candidates?: number;
  repair_best?: string;
  repair_parallel_tasks?: number;
  repair_parallel_workers_used?: number;
  stopped_after_coverage_ceiling?: boolean;
}

export type SolveProgressPhase = "loading" | "remote" | "seed" | "repair" | "search" | "refine" | "done";

export interface SolveProgress {
  phase: SolveProgressPhase;
  nodes_explored: number;
  nodes_total?: number;
  percent?: number;
  elapsed_ms: number;
  nodes_per_second?: number;
  eta_ms?: number;
  partial_solutions?: Solution[];
}

export interface Placement {
  instance_id: string;
  item_id: string;
  rotation: number;
  origin: CoordTuple;
  cells: CoordTuple[];
  star_positions: CoordTuple[];
}

export interface CraftActivation {
  result: string;
  anchor_instance: string;
  ingredient_instances: string[];
}

export interface StarActivation {
  source_instance: string;
  target_instance: string;
  star_position: CoordTuple;
  effect_text: string;
}

export interface Solution {
  layout_key?: string;
  score: SolutionScore;
  search: SearchStats;
  coverage?: CoverageBreakdown;
  coverage_groups?: CoverageBreakdown[];
  loose_star_priorities?: LooseStarPriority[];
  placements: Placement[];
  crafts: CraftActivation[];
  stars: StarActivation[];
}

export type SolverBackend = "remote" | "oci" | "local";

export interface RemoteSolveMetadata {
  backend: string;
  server_elapsed_ms?: number;
  workers?: number;
  max_nodes_applied?: number;
  max_nodes_capped?: boolean;
}
