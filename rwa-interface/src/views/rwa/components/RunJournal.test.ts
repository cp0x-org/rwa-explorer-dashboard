import { describe, expect, it } from 'vitest';

import { attrText, utcDay, utcTime } from './RunJournal';

describe('journal values', () => {
  it('scalars are text, arrays show five items and +N, objects are JSON', () => {
    expect(attrText('NVDA')).toEqual({ text: 'NVDA', full: null });
    expect(attrText(502)).toEqual({ text: '502', full: null });
    expect(attrText(null)).toEqual({ text: 'null', full: null });
    expect(attrText(true).text).toBe('true');
    expect(attrText(['a', 'b'])).toEqual({ text: 'a, b', full: null });
    expect(attrText(['a', 'b', 'c', 'd', 'e', 'f', 'g'])).toEqual({ text: 'a, b, c, d, e +2', full: 'a, b, c, d, e, f, g' });
    expect(attrText({ a: 1, b: [2] }).text).toBe('{"a":1,"b":[2]}');
  });

  it('a long value is clamped to 200 characters and the full text is kept', () => {
    const long = 'x'.repeat(500);
    const { text, full } = attrText(long);
    expect(text).toHaveLength(200);
    expect(text.endsWith('…')).toBe(true);
    expect(full).toBe(long);
  });

  it('time and day are UTC', () => {
    expect(utcTime('2026-10-20T23:59:58.123Z')).toBe('23:59:58');
    expect(utcDay('2026-10-20T23:59:58Z')).toBe('10-20');
    expect(utcDay('2026-10-21T00:00:01Z')).toBe('10-21');
  });
});
