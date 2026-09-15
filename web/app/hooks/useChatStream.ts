import { useState, useCallback } from 'react';
import { Message } from '../lib/messageProcessing';
import { formatAndCleanYaml, extractContractYaml, cleanMessageContent } from '../lib/yamlUtils';
import { api } from '~/lib/api';

interface UseChatStreamOptions {
  onError?: (message: string) => void;
  onTokenUpdate?: (count: number) => void;
  onMessageCountUpdate?: (count: number) => void;
  onConversationCreated?: (id: string) => void;
}

export const useChatStream = (options: UseChatStreamOptions = {}) => {
  const [isLoading, setIsLoading] = useState(false);

  const sendMessage = useCallback(async (
    userInput: string,
    conversationId: string | null,
    selectedSkill: string,
    messages: Message[],
    setMessages: React.Dispatch<React.SetStateAction<Message[]>>,
    setPreviewData: (data: any) => void,
    setIsPreviewOpen: (open: boolean) => void
  ) => {
    if (!userInput.trim()) return;

    const userMessage: Message = {
      id: `user-${Date.now()}`,
      role: 'user',
      content: userInput,
      timestamp: new Date(),
    };

    setMessages(prev => [...prev, userMessage]);
    setIsLoading(true);

    try {
      const response = await fetch('/api/chat/ask', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          message: userInput,
          conversation_id: conversationId,
          skill: selectedSkill,
        }),
      });

      if (!response.ok) {
        throw new Error('Failed to send message');
      }

      const reader = response.body?.getReader();
      if (!reader) {
        throw new Error('No response body');
      }

      const decoder = new TextDecoder();
      let buffer = '';
      let currentEvent = '';
      let assistantMessageId = `assistant-${Date.now()}`;

      setMessages(prev => [...prev, {
        id: assistantMessageId,
        role: 'assistant',
        content: '',
        timestamp: new Date(),
      }]);

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;

        buffer += decoder.decode(value, { stream: true });
        const lines = buffer.split('\n');
        buffer = lines.pop() || '';

        for (const line of lines) {
          if (line.startsWith('event: ')) {
            currentEvent = line.substring(7).trim();
          } else if (line.startsWith('data: ')) {
            const data = line.substring(6);
            await handleSSEEvent(
              currentEvent,
              data,
              assistantMessageId,
              setMessages,
              setPreviewData,
              setIsPreviewOpen,
              options
            );
          }
        }
      }
    } catch (error) {
      options.onError?.('Failed to send message. Please try again.');
      console.error('Error sending message:', error);
    } finally {
      setIsLoading(false);
    }
  }, [options]);

  return { sendMessage, isLoading };
};

async function handleSSEEvent(
  event: string,
  data: string,
  assistantMessageId: string,
  setMessages: React.Dispatch<React.SetStateAction<Message[]>>,
  setPreviewData: (data: any) => void,
  setIsPreviewOpen: (open: boolean) => void,
  options: UseChatStreamOptions
) {
  switch (event) {
    case 'chunk':
      setMessages(prev => prev.map(msg =>
        msg.id === assistantMessageId
          ? { ...msg, content: msg.content + data }
          : msg
      ));
      break;

    case 'conversation':
      options.onConversationCreated?.(data);
      break;

    case 'tokens':
      options.onTokenUpdate?.(parseInt(data) || 0);
      break;

    case 'message_count':
      options.onMessageCountUpdate?.(parseInt(data) || 0);
      break;

    case 'done':
      setMessages(prev => {
        return prev.map(msg => {
          if (msg.id === assistantMessageId) {
            const cleanedContent = cleanMessageContent(msg.content);
            const yamlContent = extractContractYaml(cleanedContent);

            if (yamlContent) {
              const formattedYaml = formatAndCleanYaml(yamlContent);
              const updatedContent = cleanedContent.includes('```yaml')
                ? cleanedContent.replace(/```yaml\n?([\s\S]*?)```/, `\`\`\`yaml\n${formattedYaml}\n\`\`\`\n\n`)
                : cleanedContent;

              api.previewContract({ yaml: formattedYaml })
                .then((result: any) => {
                  const contractData = { yaml: formattedYaml, response: result };
                  setMessages(msgs => msgs.map(m =>
                    m.id === assistantMessageId
                      ? { ...m, content: updatedContent, contractPreview: contractData }
                      : m
                  ));
                  setPreviewData(contractData);
                  setIsPreviewOpen(true);
                })
                .catch((err: unknown) => {
                  console.error('[Contract Preview] Failed:', err);
                  const contractData = {
                    yaml: formattedYaml,
                    response: {
                      errors: [{
                        message: 'Failed to generate graph preview',
                        details: err instanceof Error ? err.message : String(err)
                      }]
                    }
                  };
                  setMessages(msgs => msgs.map(m =>
                    m.id === assistantMessageId
                      ? { ...m, content: updatedContent, contractPreview: contractData }
                      : m
                  ));
                });

              return { ...msg, content: updatedContent };
            }

            return { ...msg, content: cleanedContent };
          }
          return msg;
        });
      });
      break;

    case 'error':
      options.onError?.(data);
      break;
  }
}
