import type { IngestionRules } from './api';

export interface TraceFilters {
  filters: string[];
  exclude: string[];
}

const sections = ['keep spans', 'drop spans'] as const;
type Section = typeof sections[number];

export function parseTraceFilters(text: string): TraceFilters {
  const result: TraceFilters = { filters: [], exclude: [] };
  let section: Section = 'keep spans';
  for (const [index, raw] of text.split(/\r?\n/).entries()) {
    const line = raw.trim();
    if (!line) continue;
    if (line.startsWith('[')) {
      const header = /^\[(.*)\]$/.exec(line)?.[1]?.toLowerCase();
      if (!sections.includes(header as Section)) throw new Error(`Line ${index + 1}: use [keep spans] or [drop spans].`);
      section = header as Section;
      continue;
    }
    const isRegex = line.startsWith('regex:');
    if (isRegex && line === 'regex:') throw new Error(`Line ${index + 1}: regex expression is empty.`);
    // Go validates regex syntax on save and preview; JavaScript uses a different dialect.
    if ((!isRegex && line.slice(0, -1).includes('*')) || /[\u0000-\u001f\u007f-\u009f]/.test(line) || new TextEncoder().encode(line).length > 256) {
      throw new Error(`Line ${index + 1}: use an exact name, trailing *, or regex: expression. Maximum 256 bytes.`);
    }
    const list = section === 'keep spans' ? result.filters : result.exclude;
    if (!list.includes(line)) list.push(line);
    if (list.length > 100) throw new Error(`[${section}] allows up to 100 patterns.`);
  }
  return result;
}

export function formatTraceFilters(rules: Pick<IngestionRules, 'filters' | 'exclude' | 'mode'>): string {
  const legacy = rules.mode === 'exclude_legacy';
  const values = {
    'keep spans': legacy ? ['*'] : rules.filters,
    'drop spans': [...(legacy ? rules.filters : []), ...(rules.exclude ?? [])],
  };
  return sections.map(section => `[${section}]\n${values[section].join('\n')}`).join('\n\n');
}
