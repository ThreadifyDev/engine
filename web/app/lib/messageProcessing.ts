import { formatAndCleanYaml, cleanMessageContent } from './yamlUtils';

export interface Message {
  id: string;
  role: 'user' | 'assistant' | 'system';
  content: string;
  timestamp: Date;
  relatedToolCall?: {
    query: string;
    response: string;
  };
  contractPreview?: {
    yaml: string;
    response: any;
  };
}

export interface RawMessage {
  id: string;
  role: string;
  content: string;
  created_at: string;
  tool_calls?: string;
}

/**
 * Process raw messages from API into UI-ready messages
 * Handles tool call linking, YAML extraction, and content cleanup
 */
export const processMessagesFromHistory = (allMessages: RawMessage[]): Message[] => {
  const processedMessages: Message[] = [];

  for (let i = 0; i < allMessages.length; i++) {
    const msg = allMessages[i];

    // Skip tool messages and empty assistant messages (tool calls)
    if (msg.role === 'tool' || (msg.role === 'assistant' && !msg.content)) {
      continue;
    }

    // Apply content cleanup
    const content = cleanMessageContent(msg.content);

    const processedMsg: Message = {
      id: msg.id,
      role: msg.role as 'user' | 'assistant' | 'system',
      content: content,
      timestamp: new Date(msg.created_at),
    };

    // Check if this assistant message was preceded by a tool call
    if (msg.role === 'assistant' && i >= 2) {
      const prevMsg = allMessages[i - 1]; // Should be tool result
      const prevPrevMsg = allMessages[i - 2]; // Should be tool call

      if (prevMsg?.role === 'tool' && prevPrevMsg?.role === 'assistant' && prevPrevMsg.tool_calls) {
        try {
          const toolCalls = JSON.parse(prevPrevMsg.tool_calls);
          const toolCall = toolCalls?.[0];
          
          if (toolCall?.function?.name === 'execute_graphql') {
            const args = JSON.parse(toolCall.function.arguments);
            processedMsg.relatedToolCall = {
              query: args.query || '',
              response: prevMsg.content || '',
            };
          } else if (toolCall?.function?.name === 'preview_contract') {
            // Reconstruct contract preview from tool call
            const args = JSON.parse(toolCall.function.arguments);
            const engineResponse = JSON.parse(prevMsg.content || '{}');
            processedMsg.contractPreview = {
              yaml: args.yaml_content || '',
              response: engineResponse,
            };
          }
        } catch (e) {
          // Ignore parsing errors
        }
      }
    }

    // Smart contract detection fallback for history
    if (!processedMsg.contractPreview) {
      const hasMarkdownYaml = processedMsg.content.includes('```yaml');
      const hasRawYaml = processedMsg.content.includes('contract_name:') && processedMsg.content.includes('steps:');
      
      if (hasMarkdownYaml || hasRawYaml) {
        let yamlText = '';
        if (hasMarkdownYaml) {
          const start = processedMsg.content.indexOf('```yaml') + 7;
          const end = processedMsg.content.indexOf('```', start);
          yamlText = (end !== -1 ? processedMsg.content.slice(start, end) : processedMsg.content.slice(start)).trim();
        } else {
          // Try to find start of YAML block (fuzzy search for first key: value pair)
          const match = processedMsg.content.match(/[a-z0-9_]+:\s*[^\n]+/i);
          if (match) {
             yamlText = processedMsg.content.slice(match.index).trim();
          }
        }

        if (yamlText.length > 20) {
          const formattedYaml = formatAndCleanYaml(yamlText);
          const engineResponseStr = processedMsg.relatedToolCall?.response;
          let engineResponse = null;
          if (engineResponseStr) {
            try { engineResponse = JSON.parse(engineResponseStr); } catch (e) {}
          }
          processedMsg.contractPreview = { yaml: formattedYaml, response: engineResponse };
        }
      }
    }

    processedMessages.push(processedMsg);
  }

  return processedMessages;
};
