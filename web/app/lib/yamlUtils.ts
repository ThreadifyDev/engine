import * as yaml from 'js-yaml';

/**
 * Robustly repairs and formats YAML content, handling common LLM mashup bugs
 */
export const formatAndCleanYaml = (input: string): string => {
  if (!input) return '';
  
  let cleaned = input.trim();
  
  // 0. Fix top-level keywords mashed together without spaces (e.g. "interactions.entry_points:")
  cleaned = cleaned.replace(/([a-z0-9\.\]])(contract_name:|version:|description:|entry_points:|parties:|steps:|transitions:|terminal_steps:)/gi, '$1\n$2');
  
  // 1. Fix list items at start: "steps:  - id: foo" or "]  - id: foo" -> "steps:\n  - id: foo"
  cleaned = cleaned.replace(/(?<=\S)(\s{2,})(-\s+[a-z_]+:)/gi, '\n$1$2');
  
  // 2. Fix properties following list items: "- id: foo    owner:" -> "- id: foo\n    owner:"
  // Match list items and force 4 spaces indentation for properties following a list item start
  cleaned = cleaned.replace(/(\s*-\s+[a-z_]+:[^\n]+?)(\s{2,})([a-z_]+:)/gi, '$1\n    $3');
  
  // 3. Fix other properties mashed together: "owner: foo    type: bar"
  // Use lookbehind to preserve original spacing which is correctly aligned
  cleaned = cleaned.replace(/(?<=\S)(\s{2,})([a-z_]+:)/gi, '\n$1$2');
  
  // 4. Fix terminal_steps mashup
  cleaned = cleaned.replace(/terminal_\s+steps:/gi, 'terminal_steps:');

  // Try parsing with js-yaml to get high-quality formatting
  try {
    const doc = yaml.load(cleaned);
    if (doc && typeof doc === 'object') {
       return yaml.dump(doc, { 
         indent: 2, 
         lineWidth: -1, // No line wrapping
         noRefs: true,
         sortKeys: false // Preserve order
       });
    }
  } catch (e) {
    console.warn('[formatAndCleanYaml] YAML parsing failed after repairs:', e);
    // Return the repaired version even if it can't be parsed
  }
  
  return cleaned;
};

/**
 * Detects and extracts YAML contract from message content
 * Returns null if no contract found
 */
export const extractContractYaml = (content: string): string | null => {
  if (!content) return null;

  // Try to extract from code block first
  const codeBlockMatch = content.match(/```yaml\n?([\s\S]*?)```/);
  if (codeBlockMatch && codeBlockMatch[1]) {
    const yamlContent = codeBlockMatch[1].trim();
    // Verify it looks like a contract
    if (yamlContent.includes('contract_name:')) {
      return yamlContent;
    }
  }

  // Fallback: check if content itself contains contract_name
  if (content.includes('contract_name:')) {
    return content.trim();
  }

  return null;
};

/**
 * Applies content cleanup for YAML formatting issues
 * Same logic used in loadConversation
 */
export const cleanMessageContent = (content: string): string => {
  let cleaned = content;
  
  // Fix mashed keywords
  cleaned = cleaned.replace(/([a-z0-9])(contract_name:)/gi, '$1\n\n$2');
  cleaned = cleaned.replace(/([a-z0-9])(version:)/gi, '$1\n$2');
  cleaned = cleaned.replace(/([a-z0-9])(description:)/gi, '$1\n$2');
  cleaned = cleaned.replace(/([a-z0-9])(steps:)/gi, '$1\n$2');
  cleaned = cleaned.replace(/([a-z0-9])(transitions:)/gi, '$1\n$2');
  cleaned = cleaned.replace(/terminal_\s+steps:/gi, 'terminal_steps:');
  
  // Ensure code block delimiters have newlines
  cleaned = cleaned.replace(/```yaml\s*([^\s\n])/g, '```yaml\n$1');
  cleaned = cleaned.replace(/:\s*```yaml/g, ':\n\n```yaml\n');
  
  return cleaned;
};
