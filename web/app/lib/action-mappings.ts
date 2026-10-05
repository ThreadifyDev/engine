import type { BrowserActionMappingRule } from './api';

// Preserve authored regex precedence. Only adjacent plain mappings can be grouped.
export function formatActionMappings(rules: BrowserActionMappingRule[]): string {
  const lines: string[] = [];
  let actions: string[] = [];
  let step = '';
  const flush = () => {
    if (actions.length) lines.push(`${actions.join(',')}=${step}`);
    actions = [];
  };
  for (const rule of rules) {
    if (rule.action.startsWith('regex:')) {
      flush();
      lines.push(`${rule.action}=${rule.step}`);
    } else {
      if (actions.length && step !== rule.step) flush();
      step = rule.step;
      actions.push(rule.action);
    }
  }
  flush();
  return lines.join('\n');
}

export function draftMappingActions(value: string): string[] {
  return value.split(/\r?\n/).flatMap(line => {
    const text = line.trim();
    const regex = text.startsWith('regex:');
    const equals = regex ? text.lastIndexOf('=') : text.indexOf('=');
    const source = equals < 0 ? text : text.slice(0, equals).trim();
    return regex ? [source] : source.split(',').map(action => action.trim());
  });
}

export function parseActionMappings(value: string, contract: string, version: number, steps: string[]): BrowserActionMappingRule[] {
  const seen = new Set<string>();
  return value.split(/\r?\n/).flatMap((line, index) => {
    const text = line.trim();
    if (!text) return [];
    const regex = text.startsWith('regex:');
    // A regex may contain '=' and commas (e.g. repetitions or literal query strings).
    const equals = regex ? text.lastIndexOf('=') : text.indexOf('=');
    if (equals < 1 || (!regex && equals !== text.lastIndexOf('=')) || equals === text.length - 1)
      throw new Error(`Line ${index + 1} must use input=contract_step.`);
    const source = text.slice(0, equals).trim();
    const actions = regex ? [source] : source.split(',').map(action => action.trim());
    const step = text.slice(equals + 1).trim();
    if (actions.some(action => !action) || !step) throw new Error(`Line ${index + 1} needs input names and a step.`);
    if (!regex && actions.some(action => action.startsWith('regex:'))) throw new Error(`Line ${index + 1}: put each regex on its own line.`);
    if (regex && source === 'regex:') throw new Error(`Line ${index + 1}: regex expression is empty.`);
    if (steps.length && !steps.includes(step)) throw new Error(`Line ${index + 1}: "${step}" is not a step in version ${version}.`);
    return actions.map(action => {
      if (seen.has(action)) throw new Error(`Line ${index + 1} repeats input "${action}".`);
      seen.add(action);
      return { action, contract, version, step };
    });
  });
}
