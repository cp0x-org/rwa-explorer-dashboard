import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';

// @ts-expect-error plain ESM without types: the mock API's metric list
import { COLUMNS, metricType } from '../../mock-api/metrics.mjs';

// The mock must accept every numeric column of the backend's history_* tables, and nothing else.
const sql = readFileSync(new URL('../../../rwa-backend/internal/db/migrations/0001_init.sql', import.meta.url), 'utf8');
const KEYS = new Set(['run_id', 'token_id']);

function numericColumns(table: string): string[] {
  const body = sql.match(new RegExp(`CREATE TABLE ${table} \\(([\\s\\S]*?)\\n\\);`))![1];
  return body
    .split('\n')
    .map((line) => line.trim().match(/^([a-z0-9_]+)\s+(INT|BIGINT|NUMERIC)\b/))
    .filter((m): m is RegExpMatchArray => !!m && !KEYS.has(m[1]))
    .map((m) => m[1]);
}

describe('mock API history metrics', () => {
  const tables: Record<string, string> = {
    summary: 'history_summary',
    underlyings: 'history_underlyings',
    tokens: 'history_tokens',
    issuers: 'history_issuers',
    opportunities: 'history_positions'
  };
  for (const [entity, table] of Object.entries(tables)) {
    it(`${entity} = numeric columns of ${table}`, () => {
      expect([...COLUMNS[entity]].sort()).toEqual(numericColumns(table).sort());
    });
  }

  it('types by name', () => {
    expect(metricType('market_cap_usd')).toBe('money');
    expect(metricType('volume_usd_1d')).toBe('money');
    expect(metricType('apy_percent')).toBe('percent');
    expect(metricType('usefulness_score')).toBe('score');
    expect(metricType('tokens_count')).toBe('count');
  });
});
