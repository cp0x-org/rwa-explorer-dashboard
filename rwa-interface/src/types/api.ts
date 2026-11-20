// Shapes of the RWA Explorer 2.0 API (docs/v2/03-api.md) other than the export, which is `RwaExport` in ./rwa.ts.
// Tests check these types against responses captured from the mock API (src/api/__fixtures__/).

// `code`: bad_request | unknown_metric | unauthorized | not_found | method_not_allowed | internal | unavailable
export interface ApiErrorBody {
  error: { code: string; message: string; metric?: string; allowed?: string[] };
}

export interface BuildInfo {
  id: number;
  run_id: number;
  generated_at: string;
  score_version: string;
  export_sha256: string;
  schema_version: number;
}

// `status`: succeeded | partial | failed | running; `mode`: refresh | add | import | rebuild;
// `trigger`: schedule | startup | manual | import | cli
// (plain strings: the server may add values, the UI shows unknown ones as they are)
export interface RunSummary {
  id: number;
  run_key: string;
  mode: string;
  trigger: string;
  status: string;
  started_at: string;
  finished_at: string | null;
  duration_seconds: number | null;
  selected_count: number;
  completed_count: number;
  failed_count: number;
  cmc_requests: number;
  portals_requests: number;
  cmc_credits: number;
  warnings_count: number;
  error: string | null;
  build_id: number | null;
  changes: RunChangesSummary | null;
}

export interface RunChangesSummary {
  new_tokens_count: number;
  removed_tokens_count: number;
  new_positions_count: number;
  gone_positions_count: number;
  eligible_delta: number;
}

export interface RunUnderlying {
  underlying_id: string;
  ticker: string;
  status: string;
  error: string | null;
  duration_seconds: number | null;
}

export interface RunChangesDetail {
  new_tokens: number[];
  removed_tokens: number[];
  new_issuers: string[];
  new_positions_count: number;
  gone_positions_count: number;
  eligible_delta: number;
  details: Record<string, unknown>;
}

export interface RunDetail extends Omit<RunSummary, 'changes'> {
  warnings: string[];
  underlyings: RunUnderlying[];
  changes: RunChangesDetail | null;
}

export interface RunsResponse {
  items: RunSummary[];
  next_cursor: string | null;
}

export type LastRun = Pick<
  RunSummary,
  'id' | 'run_key' | 'mode' | 'trigger' | 'status' | 'started_at' | 'finished_at' | 'completed_count' | 'failed_count' | 'error'
>;

export interface StatusResponse {
  current_build: BuildInfo | null; // null before the first build
  last_run: LastRun | null;
  next_run_at: string | null;
  schedule: { cron: string | null; interval: string | null };
  collector_version: string | null;
  api_version: string;
}

export type HistoryEntity = 'summary' | 'underlyings' | 'tokens' | 'issuers' | 'opportunities';
export type HistoryStep = 'run' | 'day' | 'week';
export type MetricType = 'money' | 'percent' | 'count' | 'score';

export interface HistoryPoint {
  run_id: number;
  observed_at: string;
  values: Record<string, string | number | null>;
}

export interface HistoryResponse {
  entity: HistoryEntity;
  id: string | null;
  metrics: string[];
  step: HistoryStep;
  points: HistoryPoint[];
  truncated: boolean;
}

export interface HistoryMetric {
  name: string;
  type: MetricType;
}

export interface HistoryMetricsResponse {
  entities: Record<HistoryEntity, HistoryMetric[]>;
}
