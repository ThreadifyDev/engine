import React, { useState, useRef, useEffect } from 'react';
import { Send, Bot, User, Loader2, Trash2, ChevronDown, Search, Code, Copy, Check, Plus, X, AlertTriangle } from 'lucide-react';
import { api } from '~/lib/api';
import ReactMarkdown from 'react-markdown';
import ContractGraphView from './ContractGraphView';

interface Message {
  id: string;
  role: 'user' | 'assistant' | 'tool';
  content: string;
  toolCalls?: any[];
  toolCallId?: string;
  timestamp: Date;
  hidden?: boolean;
  relatedToolCall?: {
    query: string;
    response: string;
  };
  contractPreview?: {
    yaml: string;
    response: any;
  };
}

interface Conversation {
  id: string;
  title: string;
  created_at: string;
  updated_at: string;
}

type Skill = 'support' | 'operations' | 'business';

export default function ThreadChat() {
  const [messages, setMessages] = useState<Message[]>([]);
  const [input, setInput] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const [conversationId, setConversationId] = useState<string | null>(null);
  const [conversations, setConversations] = useState<Conversation[]>([]);
  const [isDropdownOpen, setIsDropdownOpen] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');
  const [selectedSkill, setSelectedSkill] = useState<Skill>('support');
  const [tokenCount, setTokenCount] = useState(0);
  const [messageCount, setMessageCount] = useState(0);
  const [expandedQueries, setExpandedQueries] = useState<Set<string>>(new Set());
  const [limitError, setLimitError] = useState<string | null>(null);
  const [isGeneratingSummary, setIsGeneratingSummary] = useState(false);
  const [copiedThreadId, setCopiedThreadId] = useState<string | null>(null);
  
  // Contract Preview State
  const [previewData, setPreviewData] = useState<{ yaml: string, response: any } | null>(null);
  const [isPreviewOpen, setIsPreviewOpen] = useState(false);
  const [isCreatingContract, setIsCreatingContract] = useState(false);
  const [editedYaml, setEditedYaml] = useState<string>('');
  const [isUpdatingPreview, setIsUpdatingPreview] = useState(false);
  const [activeTab, setActiveTab] = useState<'diagram' | 'yaml'>('diagram');
  
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const dropdownRef = useRef<HTMLDivElement>(null);

  // Load conversations on mount and restore last conversation
  useEffect(() => {
    loadConversations();
    
    // Restore last conversation from localStorage
    const lastConversationId = localStorage.getItem('lastConversationId');
    if (lastConversationId) {
      loadConversation(lastConversationId);
    }
  }, []);

  // Close dropdown when clicking outside
  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (dropdownRef.current && !dropdownRef.current.contains(event.target as Node)) {
        setIsDropdownOpen(false);
        setSearchQuery('');
      }
    };
    document.addEventListener('mousedown', handleClickOutside);
    return () => document.removeEventListener('mousedown', handleClickOutside);
  }, []);

  const loadConversations = async () => {
    try {
      const response = await api.getChatConversations();
      setConversations(response.conversations || []);
    } catch (error) {
      // Silent fail - user will see empty conversation list
    }
  };

  const loadConversation = async (convId: string) => {
    try {
      const response = await api.getChatMessageHistory(convId);
      const allMessages = response.messages;
      
      // Process messages and link tool calls to their responses
      const processedMessages: Message[] = [];
      
      for (let i = 0; i < allMessages.length; i++) {
        const msg = allMessages[i];
        
        // Skip tool messages and empty assistant messages (tool calls)
        if (msg.role === 'tool' || (msg.role === 'assistant' && !msg.content)) {
          continue;
        }
        
        const processedMsg: Message = {
          id: msg.id,
          role: msg.role,
          content: msg.content,
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
        
        processedMessages.push(processedMsg);
      }
      
      setMessages(processedMessages);
      setConversationId(convId);
      setIsDropdownOpen(false);
      setSearchQuery('');
      
      // Save to localStorage for auto-restore on next visit
      localStorage.setItem('lastConversationId', convId);
    } catch (error) {
      // Silent fail - conversation won't load
    }
  };

  const startNewConversation = () => {
    setMessages([]);
    setConversationId(null);
    setTokenCount(0);
    setMessageCount(0);
    setLimitError(null);
    setPreviewData(null);
    setIsPreviewOpen(false);
    
    // Clear last conversation from localStorage
    localStorage.removeItem('lastConversationId');
  };

  const continueWithContext = async () => {
    if (!conversationId) return;
    
    setIsGeneratingSummary(true);
    
    try {
      const response = await api.continueConversation(conversationId);
      
      // Switch to new conversation
      setConversationId(response.conversation_id);
      setMessages([]);
      setTokenCount(0);
      setMessageCount(0);
      setLimitError(null);
      
      // Save new conversation to localStorage
      localStorage.setItem('lastConversationId', response.conversation_id);
      
      // Reload conversations list
      loadConversations();
    } catch (error) {
      alert('Failed to continue conversation with context');
    } finally {
      setIsGeneratingSummary(false);
    }
  };

  const deleteConversation = async (convId: string, e: React.MouseEvent) => {
    e.stopPropagation(); // Prevent loading the conversation
    
    if (!confirm('Are you sure you want to delete this conversation?')) {
      return;
    }

    try {
      await api.deleteChatConversation(convId);
      
      // If we deleted the current conversation, clear it
      if (conversationId === convId) {
        setMessages([]);
        setConversationId(null);
      }
      
      // Refresh conversation list
      loadConversations();
    } catch (error) {
      alert('Failed to delete conversation');
    }
  };

  const scrollToBottom = () => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  };

  useEffect(() => {
    scrollToBottom();
  }, [messages]);

  // Convert thread IDs (UUIDs) to clickable links
  const linkifyThreadIds = (text: string) => {
    const uuidRegex = /\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b/gi;
    const parts = text.split(uuidRegex);
    const matches = text.match(uuidRegex) || [];
    
    return parts.reduce((acc, part, i) => {
      acc.push(part);
      if (matches[i]) {
        acc.push(
          <a
            key={`link-${i}`}
            href={`/u/threads/${matches[i]}`}
            target="_blank"
            rel="noopener noreferrer"
            className="text-blue-600 hover:text-blue-800 underline"
            onClick={(e) => e.stopPropagation()}
          >
            {matches[i]}
          </a>
        );
      }
      return acc;
    }, [] as (string | JSX.Element)[]);
  };

  const handleKeyDown = (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSubmit(e);
    }
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!input.trim() || isLoading) return;

    const userMessage: Message = {
      id: Date.now().toString(),
      role: 'user',
      content: input,
      timestamp: new Date(),
    };

    setMessages(prev => [...prev, userMessage]);
    const userInput = input;
    setInput('');
    setIsLoading(true);

    try {
      const token = localStorage.getItem('auth_token');
      const apiUrl = (window as any).__ENV__?.API_URL || 'http://localhost:3001';
      
      const response = await fetch(`${apiUrl}/api/chat/ask`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'Authorization': `Bearer ${token}`,
        },
        body: JSON.stringify({
          message: userInput,
          conversation_id: conversationId,
        }),
      });

      if (!response.ok) {
        const errorData = await response.json().catch(() => ({ error: 'Failed to get response' }));
        
        // Check if it's a limit error
        if (errorData.error && (errorData.error.includes('maximum') || errorData.error.includes('limit'))) {
          setLimitError(errorData.error);
          setIsLoading(false);
          return;
        }
        
        throw new Error(errorData.error || 'Failed to get response');
      }

      const reader = response.body?.getReader();
      const decoder = new TextDecoder();
      let assistantContent = '';
      let assistantMessageId = (Date.now() + 1).toString();

      // Add empty assistant message that we'll update
      setMessages(prev => [...prev, {
        id: assistantMessageId,
        role: 'assistant',
        content: '',
        timestamp: new Date(),
      }]);

      if (reader) {
        let buffer = '';
        let currentEvent = '';
        
        while (true) {
          const { done, value } = await reader.read();
          if (done) break;

          buffer += decoder.decode(value, { stream: true });
          const lines = buffer.split('\n');
          
          // Keep the last incomplete line in the buffer
          buffer = lines.pop() || '';

          for (const line of lines) {
            if (!line.trim()) continue; // Skip empty lines
            
            if (line.startsWith('event:')) {
              // Handle both 'event:chunk' and 'event: chunk'
              currentEvent = line.slice(6).trim();
            } else if (line.startsWith('data:')) {
              // Get data after 'data:' - keep everything after the colon
              // The space after 'data:' is part of the SSE format, but the content itself has spaces
              const data = line.slice(5); // Keep 'data:' prefix (5 chars) - includes leading space if present
              
              // Handle different event types
              if (currentEvent === 'chunk') {
                // Text content from LLM
                assistantContent += data;
                setMessages(prev => prev.map(msg => 
                  msg.id === assistantMessageId 
                    ? { ...msg, content: assistantContent }
                    : msg
                ));
              } else if (currentEvent === 'conversation') {
                // Conversation ID
                if (!conversationId) {
                  setConversationId(data);
                  loadConversations();
                  // Save new conversation to localStorage
                  localStorage.setItem('lastConversationId', data);
                }
              } else if (currentEvent === 'tokens') {
                // Token count
                setTokenCount(parseInt(data) || 0);
              } else if (currentEvent === 'message_count') {
                // Message count
                setMessageCount(parseInt(data) || 0);
              } else if (currentEvent === 'system') {
                // System messages (like "Querying Threadify Engine...")
                // Optionally show system message in UI
                if (data.includes('Querying')) {
                  setMessages(prev => prev.map(msg => 
                    msg.id === assistantMessageId 
                      ? { ...msg, content: assistantContent + '\n\n_' + data + '_' }
                      : msg
                  ));
                }
              } else if (currentEvent === 'tool_call') {
                // Tool call information (GraphQL query and response)
                try {
                  const toolCallData = JSON.parse(data.trim());
                  setMessages(prev => prev.map(msg => 
                    msg.id === assistantMessageId 
                      ? { 
                          ...msg, 
                          relatedToolCall: {
                            query: toolCallData.query || '',
                            response: toolCallData.response || '',
                          }
                        }
                      : msg
                  ));
                } catch (e) {
                  // Ignore parsing errors
                }
              } else if (currentEvent === 'contract_preview') {
                try {
                  const previewCallData = JSON.parse(data.trim());
                  console.log('[CONTRACT_PREVIEW] Raw data:', previewCallData);
                  const engineResponse = JSON.parse(previewCallData.response);
                  console.log('[CONTRACT_PREVIEW] Parsed engine response:', engineResponse);
                  
                  const contractData = {
                    yaml: previewCallData.yaml,
                    response: engineResponse
                  };
                  
                  setPreviewData(contractData);
                  setIsPreviewOpen(true);
                  
                  // Store contract preview in the assistant message for later viewing
                  setMessages(prev => prev.map(msg => 
                    msg.id === assistantMessageId 
                      ? { ...msg, contractPreview: contractData }
                      : msg
                  ));
                } catch (e) {
                  console.error('[CONTRACT_PREVIEW] Parse error:', e);
                }
              } else if (currentEvent === 'done') {
                // Stream complete
                break;
              }
              
              // Don't reset currentEvent - it persists until next event: line
            }
          }
        }
      }
    } catch (error) {
      const errorMessage: Message = {
        id: (Date.now() + 1).toString(),
        role: 'assistant',
        content: 'Sorry, I encountered an error. Please try again.',
        timestamp: new Date(),
      };
      setMessages(prev => [...prev, errorMessage]);
    } finally {
      setIsLoading(false);
    }
  };

  const handleCreateContract = async () => {
    if (!previewData || previewData.response?.errors?.length > 0) return;
    
    setIsCreatingContract(true);
    try {
      const result = await api.createContract({
        name: previewData.response?.contract?.contract_name || 'unnamed_contract',
        yaml: previewData.yaml,
      });
      
      alert('Contract created successfully!');
      setIsPreviewOpen(false);
      setPreviewData(null);
      
    } catch (err: any) {
      console.error('Create contract error:', err);
      alert(err.message || 'An error occurred while creating the contract');
    } finally {
      setIsCreatingContract(false);
    }
  };

  return (
    <div className="flex h-full relative overflow-hidden">
      {/* Chat Area - Takes 60% width when preview is open, full width when closed */}
      <div className={`flex flex-col h-full transition-all duration-300 ${isPreviewOpen ? 'w-3/5 border-r border-gray-200' : 'w-full'}`}>
        {/* Header with Custom Conversation Dropdown */}
        <div className="border-b border-gray-200 px-4 py-3 bg-white">
        <div className="flex items-center gap-3">
          <div className="flex items-center gap-2">
            {/* <Bot className="w-5 h-5 text-gray-900" /> */}
            <h2 className="text-sm font-semibold text-gray-900">Threadify AI</h2>
          </div>
          <div className="flex-1 relative" ref={dropdownRef}>
            {/* Dropdown Trigger */}
            <button
              onClick={() => setIsDropdownOpen(!isDropdownOpen)}
              className="w-full text-left text-xs border border-gray-300 rounded-md px-3 py-2 focus:outline-none focus:ring-2 focus:ring-gray-900 focus:border-transparent bg-white hover:bg-gray-50 transition-colors flex items-center justify-between"
            >
              <span className="truncate">
                {conversationId 
                  ? conversations.find(c => c.id === conversationId)?.title || 'Select conversation'
                  : 'New conversation'}
              </span>
              <ChevronDown className={`w-4 h-4 text-gray-400 transition-transform ${isDropdownOpen ? 'rotate-180' : ''}`} />
            </button>

            {/* Custom Dropdown */}
            {isDropdownOpen && (
              <div className="absolute top-full left-0 right-0 mt-1 bg-white border border-gray-200 rounded-md shadow-lg z-50 max-h-96 overflow-hidden flex flex-col">
                {/* Search Box */}
                <div className="p-2 border-b border-gray-200">
                  <div className="relative">
                    <Search className="absolute left-2 top-1/2 -translate-y-1/2 w-4 h-4 text-gray-400" />
                    <input
                      type="text"
                      value={searchQuery}
                      onChange={(e) => setSearchQuery(e.target.value)}
                      placeholder="Search conversations..."
                      className="w-full pl-8 pr-3 py-1.5 text-xs border border-gray-300 rounded focus:outline-none focus:ring-2 focus:ring-gray-900 focus:border-transparent"
                      onClick={(e) => e.stopPropagation()}
                    />
                  </div>
                </div>

                {/* New Conversation Option */}
                <button
                  onClick={() => {
                    startNewConversation();
                    setIsDropdownOpen(false);
                    setSearchQuery('');
                  }}
                  className="px-3 py-2 text-xs text-left hover:bg-gray-50 border-b border-gray-100 font-medium text-gray-900"
                >
                  + New conversation
                </button>

                {/* Conversation List */}
                <div className="overflow-y-auto">
                  {conversations
                    .filter(conv => 
                      conv.title.toLowerCase().includes(searchQuery.toLowerCase())
                    )
                    .map((conv) => (
                      <div
                        key={conv.id}
                        className={`group flex items-center justify-between px-3 py-2 hover:bg-gray-50 cursor-pointer ${
                          conversationId === conv.id ? 'bg-gray-100' : ''
                        }`}
                      >
                        <div 
                          onClick={() => loadConversation(conv.id)}
                          className="flex-1 min-w-0"
                        >
                          <div className="text-xs font-medium text-gray-900 truncate">
                            {conv.title}
                          </div>
                          <div className="text-xs text-gray-500">
                            {new Date(conv.updated_at).toLocaleDateString()}
                          </div>
                        </div>
                        <button
                          onClick={(e) => deleteConversation(conv.id, e)}
                          className="ml-2 p-1 rounded hover:bg-red-50 opacity-0 group-hover:opacity-100 transition-opacity"
                          title="Delete conversation"
                        >
                          <Trash2 className="w-3.5 h-3.5 text-red-600" />
                        </button>
                      </div>
                    ))}
                  {conversations.filter(conv => 
                    conv.title.toLowerCase().includes(searchQuery.toLowerCase())
                  ).length === 0 && searchQuery && (
                    <div className="px-3 py-4 text-xs text-gray-500 text-center">
                      No conversations found
                    </div>
                  )}
                </div>
              </div>
            )}
          </div>
        </div>
      </div>

      {/* Messages */}
      <div className="flex-1 overflow-y-auto p-4 space-y-4">
        {messages.length === 0 && (
          <div className="text-center py-12">
            <Bot className="w-12 h-12 text-gray-300 mx-auto mb-3" />
            <p className="text-sm text-gray-500">Ask Threadify AI any question about your threads</p>
            <p className="text-xs text-gray-400 mt-1">
              Example: "Show me failed threads from the last 24 hours"
            </p>
          </div>
        )}

        {messages.map((message) => (
          <div
            key={message.id}
            className={`flex gap-3 ${message.role === 'user' ? 'justify-end' : 'justify-start'}`}
          >
            {message.role === 'assistant' && (
              <div className="flex-shrink-0 w-8 h-8 rounded-full bg-gray-200 flex items-center justify-center">
                <Bot className="w-4 h-4 text-gray-900" />
              </div>
            )}
            <div
              className={`max-w-[80%] rounded-lg px-4 py-2 ${
                message.role === 'user'
                  ? 'bg-gray-900 text-white'
                  : 'bg-gray-100 text-gray-900'
              }`}
            >
              <div className="text-sm break-words prose prose-sm max-w-none prose-p:my-1 prose-strong:font-semibold prose-strong:text-gray-900">
                {message.role === 'assistant' ? (
                  <ReactMarkdown
                    components={{
                      code: ({ node, className, children, ...props }) => {
                        const text = String(children);
                        const uuidRegex = /\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b/gi;
                        
                        const match = /language-(\w+)/.exec(className || '');
                        const isInline = !match && !text.includes('\n');
                        
                        if (isInline && uuidRegex.test(text)) {
                          return (
                            <a
                              href={`/u/threads/${text.trim()}`}
                              target="_blank"
                              rel="noopener noreferrer"
                              className="text-blue-600 hover:text-blue-800 underline font-mono text-xs bg-gray-200 px-1 rounded"
                            >
                              {`View Thread ${text.substring(text.length - 8, text.length - 1)}`}
                            </a>
                          );
                        }
                        
                        return isInline ? (
                          <code className="bg-gray-200 px-1 rounded" {...props}>{children}</code>
                        ) : (
                          <code className="block bg-gray-200 p-2 rounded" {...props}>{children}</code>
                        );
                      },
                      p: ({ children }) => {
                        const processText = (node: any): any => {
                          if (typeof node === 'string') {
                            const uuidRegex = /\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b/gi;
                            const parts = node.split(uuidRegex);
                            const matches = node.match(uuidRegex) || [];
                            
                            if (matches.length === 0) return node;
                            
                            return parts.reduce((acc: any[], part: string, i: number) => {
                              if (part) acc.push(part);
                              if (matches[i]) {
                                acc.push(
                                  <span key={`uuid-wrapper-${i}`} className="inline-flex items-center gap-1">
                                    <a
                                      href={`/u/threads/${matches[i]}`}
                                      target="_blank"
                                      rel="noopener noreferrer"
                                      className="text-gray-600 hover:text-gray-800 underline"
                                      title={matches[i]}
                                    >
                                      {matches[i].slice(-12)}
                                    </a>
                                    <button
                                      onClick={(e) => {
                                        e.preventDefault();
                                        navigator.clipboard.writeText(matches[i]);
                                        setCopiedThreadId(matches[i]);
                                        setTimeout(() => setCopiedThreadId(null), 2000);
                                      }}
                                      className="inline-flex items-center justify-center w-4 h-4 text-gray-500 hover:text-gray-700 transition-colors"
                                      title="Copy full thread ID"
                                    >
                                      {copiedThreadId === matches[i] ? (
                                        <Check className="w-3 h-3 text-green-600" />
                                      ) : (
                                        <Copy className="w-3 h-3" />
                                      )}
                                    </button>
                                  </span>
                                );
                              }
                              return acc;
                            }, []);
                          }
                          return node;
                        };
                        
                        return <p>{React.Children.map(children, processText)}</p>;
                      },
                      strong: ({ children }) => {
                        const processText = (node: any): any => {
                          if (typeof node === 'string') {
                            const uuidRegex = /\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b/gi;
                            const parts = node.split(uuidRegex);
                            const matches = node.match(uuidRegex) || [];
                            
                            if (matches.length === 0) return <strong>{node}</strong>;
                            
                            return (
                              <strong>
                                {parts.reduce((acc: any[], part: string, i: number) => {
                                  if (part) acc.push(part);
                                  if (matches[i]) {
                                    acc.push(
                                      <span key={`uuid-wrapper-${i}`} className="inline-flex items-center gap-1">
                                        <a
                                          href={`/u/threads/${matches[i]}`}
                                          target="_blank"
                                          rel="noopener noreferrer"
                                          className="text-gray-600 hover:text-gray-800 underline font-bold"
                                          title={matches[i]}
                                        >
                                          {matches[i].slice(-12)}
                                        </a>
                                        <button
                                          onClick={(e) => {
                                            e.preventDefault();
                                            navigator.clipboard.writeText(matches[i]);
                                            setCopiedThreadId(matches[i]);
                                            setTimeout(() => setCopiedThreadId(null), 2000);
                                          }}
                                          className="inline-flex items-center justify-center w-4 h-4 text-gray-500 hover:text-gray-700 transition-colors"
                                          title="Copy full thread ID"
                                        >
                                          {copiedThreadId === matches[i] ? (
                                            <Check className="w-3 h-3 text-green-600" />
                                          ) : (
                                            <Copy className="w-3 h-3" />
                                          )}
                                        </button>
                                      </span>
                                    );
                                  }
                                  return acc;
                                }, [])}
                              </strong>
                            );
                          }
                          return <strong>{node}</strong>;
                        };
                        
                        return <>{React.Children.map(children, processText)}</>;
                      },
                    }}
                  >
                    {message.content}
                  </ReactMarkdown>
                ) : (
                  <div className="whitespace-pre-wrap">{message.content}</div>
                )}
              </div>
              
              {/* Timestamp and Action Buttons */}
              <div className="flex items-center justify-between mt-1">
                <p className="text-xs opacity-60">
                  {message.timestamp.toLocaleTimeString()}
                </p>
                <div className="flex items-center gap-2">
                  {message.contractPreview && (
                    <button
                      onClick={() => {
                        setPreviewData(message.contractPreview!);
                        setIsPreviewOpen(true);
                      }}
                      className="flex items-center gap-1 text-xs text-gray-600 hover:text-gray-800 transition-colors font-medium"
                    >
                      <Code className="w-3 h-3" />
                      View Contract
                    </button>
                  )}
                  {message.relatedToolCall && (
                    <button
                      onClick={() => {
                        const newExpanded = new Set(expandedQueries);
                        if (newExpanded.has(message.id)) {
                          newExpanded.delete(message.id);
                        } else {
                          newExpanded.add(message.id);
                        }
                        setExpandedQueries(newExpanded);
                      }}
                      className="flex items-center gap-1 text-xs text-gray-600 hover:text-gray-900 transition-colors"
                    >
                      <Code className="w-3 h-3" />
                      {expandedQueries.has(message.id) ? 'Hide' : 'View'} Query
                    </button>
                  )}
                </div>
              </div>
              
              {/* Expandable Query Section */}
              {message.relatedToolCall && expandedQueries.has(message.id) && (
                <div className="mt-2 pt-2 border-t border-gray-200 space-y-2">
                  <div>
                    <div className="text-xs font-semibold text-gray-700 mb-1">Query:</div>
                    <pre className="bg-gray-800 text-green-400 p-2 rounded text-xs overflow-x-auto">
                      <code>{message.relatedToolCall.query}</code>
                    </pre>
                  </div>
                  <div>
                    <div className="text-xs font-semibold text-gray-700 mb-1">Response:</div>
                    <pre className="bg-gray-800 text-blue-300 p-2 rounded text-xs overflow-x-auto max-h-48 overflow-y-auto">
                      <code>{JSON.stringify(JSON.parse(message.relatedToolCall.response), null, 2)}</code>
                    </pre>
                  </div>
                </div>
              )}
            </div>
            {message.role === 'user' && (
              <div className="flex-shrink-0 w-8 h-8 rounded-full bg-gray-900 flex items-center justify-center">
                <User className="w-4 h-4 text-white" />
              </div>
            )}
          </div>
        ))}

        {isLoading && (
          <div className="flex gap-3 justify-start">
            <div className="flex-shrink-0 w-8 h-8 rounded-full bg-gray-200 flex items-center justify-center">
              <Bot className="w-4 h-4 text-gray-900" />
            </div>
            <div className="bg-gray-100 rounded-lg px-4 py-2">
              <Loader2 className="w-4 h-4 animate-spin text-gray-600" />
            </div>
          </div>
        )}

        <div ref={messagesEndRef} />
      </div>

      {/* Input */}
      <div className="border-t border-gray-200 p-4 bg-white">
        {/* Limit Error Message */}
        {limitError && (
          <div className="mb-4 p-4 bg-red-50 border border-red-200 rounded-lg">
            <p className="text-sm text-red-800 font-medium mb-3">{limitError}</p>
            
            {isGeneratingSummary ? (
              <div className="flex items-center justify-center gap-2 py-2">
                <Loader2 className="w-4 h-4 animate-spin text-gray-900" />
                <span className="text-sm text-gray-600 font-medium">Preparing context summary...</span>
              </div>
            ) : (
              <div className="flex gap-2">
                <button
                  onClick={continueWithContext}
                  className="flex-1 px-4 py-2 bg-gray-800 text-white text-sm font-medium rounded-lg hover:bg-gray-900 transition-colors"
                >
                  New chat (keep context)
                </button>
                <button
                  onClick={startNewConversation}
                  className="flex-1 px-4 py-2 bg-gray-50 text-gray-900 text-sm font-medium rounded-lg hover:bg-gray-300 transition-colors"
                >
                  New chat (fresh start)
                </button>
              </div>
            )}
          </div>
        )}
        
        <form onSubmit={handleSubmit} className="space-y-2">
          <div className="flex gap-2 items-end">
            <textarea
              value={input}
              onChange={(e) => setInput(e.target.value)}
              onKeyDown={handleKeyDown}
              placeholder="Ask about your threads..."
              className="flex-1 px-4 py-3 border border-gray-300 rounded-lg focus:outline-none focus:ring-2 focus:ring-gray-900 focus:border-transparent text-sm resize-none min-h-[44px] max-h-[200px] overflow-y-auto"
              disabled={isLoading || !!limitError}
              rows={1}
              style={{
                height: 'auto',
                minHeight: '44px',
              }}
              onInput={(e) => {
                const target = e.target as HTMLTextAreaElement;
                target.style.height = 'auto';
                target.style.height = Math.min(target.scrollHeight, 200) + 'px';
              }}
            />
            <button
              type="submit"
              disabled={isLoading || !input.trim()}
              className="px-4 py-3 bg-gray-900 text-white rounded-lg hover:bg-gray-800 disabled:opacity-50 disabled:cursor-not-allowed transition-colors flex-shrink-0"
            >
              <Send className="w-4 h-4" />
            </button>
          </div>
          <div className="flex items-center justify-between gap-4">
            <div className="flex items-center gap-2">
              <select
                value={selectedSkill}
                onChange={(e) => setSelectedSkill(e.target.value as Skill)}
                className="px-2 py-1 text-xs border border-gray-300 rounded hover:bg-gray-50 focus:outline-none focus:ring-1 focus:ring-gray-900 bg-white text-gray-700"
              >
                <option value="support">Support</option>
                <option value="operations">Operations</option>
                <option value="business">Business</option>
              </select>
              <span className="text-xs text-gray-500">
                {selectedSkill === 'support' && 'Customer troubleshooting'}
                {selectedSkill === 'operations' && 'Workflow monitoring & reliability'}
                {selectedSkill === 'business' && 'Analytics & insights'}
              </span>
            </div>
            {conversationId && (tokenCount > 0 || messageCount > 0) && (
              <div className="flex items-center gap-1.5 text-[10px] text-gray-400">
                <div className="flex items-center gap-1">
                  <div className="w-12 h-0.5 bg-gray-200 rounded-full overflow-hidden">
                    <div 
                      className={`h-full transition-all ${tokenCount > 180000 ? 'bg-red-500' : tokenCount > 150000 ? 'bg-yellow-500' : 'bg-green-500'}`}
                      style={{ width: `${Math.min((tokenCount / 200000) * 100, 100)}%` }}
                    />
                  </div>
                  <span className="font-mono">{(tokenCount / 1000).toFixed(0)}k</span>
                </div>
                <span>•</span>
                <span className="font-mono">{messageCount}/50</span>
              </div>
            )}
          </div>
        </form>
      </div>
    </div>

    {/* Right Panel: Contract Preview */}
    {isPreviewOpen && previewData && (
      <div className="w-2/5 h-full flex flex-col bg-gray-50 border-l border-gray-200 overflow-hidden animate-in slide-in-from-right-8 duration-300">
        <div className="flex items-center justify-between px-4 py-3 border-b border-gray-200 bg-white shadow-sm">
          <div className="flex items-center gap-2">
            <Code className="w-5 h-5 text-gray-600" />
            <h2 className="text-sm font-semibold text-gray-900">Contract Preview</h2>
          </div>
          <div className="flex items-center gap-2">
            <button
              onClick={() => setIsPreviewOpen(false)}
              className="p-1 hover:bg-gray-100 rounded text-gray-500 hover:text-gray-700 transition-colors"
              title="Close preview"
            >
              <X className="w-4 h-4" />
            </button>
          </div>
        </div>
        
        {/* Validation Errors Banner */}
        {previewData.response?.errors && previewData.response.errors.length > 0 && (
          <div className="bg-red-50 border-b border-red-200 p-3">
            <div className="flex items-start gap-2">
              <AlertTriangle className="w-4 h-4 text-red-600 mt-0.5 flex-shrink-0" />
              <div>
                <h3 className="text-sm font-medium text-red-800">Validation Errors</h3>
                <ul className="mt-1 text-xs text-red-700 list-disc list-inside pl-4 space-y-0.5">
                  {previewData.response.errors.map((err: any, idx: number) => (
                    <li key={idx}>{err}</li>
                  ))}
                </ul>
              </div>
            </div>
          </div>
        )}

        {/* Tab Headers */}
        <div className="flex border-b border-gray-200 bg-white">
          <button
            onClick={() => setActiveTab('diagram')}
            className={`px-4 py-2.5 text-xs font-medium transition-colors ${
              activeTab === 'diagram'
                ? 'text-gray-900 border-b-2 border-gray-900'
                : 'text-gray-500 hover:text-gray-700'
            }`}
          >
            Graph Preview
          </button>
          <button
            onClick={() => setActiveTab('yaml')}
            className={`px-4 py-2.5 text-xs font-medium transition-colors ${
              activeTab === 'yaml'
                ? 'text-gray-900 border-b-2 border-gray-900'
                : 'text-gray-500 hover:text-gray-700'
            }`}
          >
            YAML Source
          </button>
        </div>

        {/* Tab Content */}
        <div className="flex-1 overflow-hidden">
          {activeTab === 'diagram' && (
            <div className="h-full">
              {(() => {
                if (previewData.response?.errors && previewData.response.errors.length > 0) {
                  return (
                    <div className="flex items-center justify-center h-full bg-white text-red-500 text-sm">
                      Cannot render graph due to validation errors.
                    </div>
                  );
                }
                
                if (previewData.response?.graph) {
                  // Unwrap nested graph structure: response.graph.graph -> response.graph
                  const unwrappedData = {
                    graph: previewData.response.graph.graph || previewData.response.graph,
                    transitions: previewData.response.graph.transitions,
                    parties: previewData.response.graph.parties,
                  };
                  return (
                    <div className="h-full w-full">
                      <ContractGraphView
                        key={JSON.stringify(unwrappedData.graph.nodes)}
                        contractName="Preview"
                        version={1}
                        graphData={unwrappedData}
                      />
                    </div>
                  );
                }
                
                return (
                  <div className="flex items-center justify-center h-full bg-white text-gray-500 text-sm">
                    No graph data available.
                  </div>
                );
              })()}
            </div>
          )}

          {activeTab === 'yaml' && (
            <div className="h-full flex flex-col">
              <div className="flex items-center justify-between p-3 bg-gray-50 border-b border-gray-200">
                <span className="text-xs font-medium text-gray-700">Edit YAML and click Update to preview changes</span>
                <div className="flex items-center gap-2">
                  <button
                    onClick={() => {
                      navigator.clipboard.writeText(editedYaml || previewData.yaml);
                      setCopiedThreadId('yaml-copy');
                      setTimeout(() => setCopiedThreadId(null), 2000);
                    }}
                    className="p-1.5 bg-gray-700 hover:bg-gray-600 text-white rounded transition-colors"
                    title="Copy YAML"
                  >
                    {copiedThreadId === 'yaml-copy' ? <Check className="w-3.5 h-3.5" /> : <Copy className="w-3.5 h-3.5" />}
                  </button>
                  <button
                    onClick={async () => {
                      setIsUpdatingPreview(true);
                      try {
                        const yamlContent = editedYaml || previewData.yaml;
                        const result = await api.previewContract({ yaml: yamlContent });
                        setPreviewData({
                          yaml: yamlContent,
                          response: result,
                        });
                        setActiveTab('diagram');
                      } catch (err) {
                        console.error('Preview error:', err);
                        alert(`Failed to update preview: ${err instanceof Error ? err.message : 'Unknown error'}`);
                      } finally {
                        setIsUpdatingPreview(false);
                      }
                    }}
                    disabled={isUpdatingPreview}
                    className="flex items-center gap-1.5 px-3 py-1.5 bg-indigo-600 hover:bg-indigo-700 text-white text-xs font-medium rounded transition-colors disabled:opacity-50"
                  >
                    {isUpdatingPreview ? (
                      <Loader2 className="w-3.5 h-3.5 animate-spin" />
                    ) : (
                      <>
                        <Code className="w-3.5 h-3.5" />
                        Update Preview
                      </>
                    )}
                  </button>
                </div>
              </div>
              <textarea
                value={editedYaml || previewData.yaml}
                onChange={(e) => setEditedYaml(e.target.value)}
                className="flex-1 bg-gray-900 text-gray-100 p-4 font-mono text-xs resize-none focus:outline-none focus:ring-2 focus:ring-indigo-500"
                spellCheck={false}
              />
            </div>
          )}
        </div>

        {/* Footer Actions */}
        <div className="p-4 bg-white border-t border-gray-200 shadow-sm flex justify-end">
          <button
            onClick={handleCreateContract}
            disabled={isCreatingContract || (previewData.response?.errors && previewData.response.errors.length > 0)}
            className="flex items-center gap-1.5 px-4 py-2 border border-gray-600 bg-gray-100 text-black text-sm font-medium rounded-md hover:bg-gray-200 disabled:opacity-50 disabled:cursor-not-allowed transition-colors shadow-sm"
          >
            {isCreatingContract ? (
              <Loader2 className="w-4 h-4 animate-spin" />
            ) : (
              <Plus className="w-4 h-4" />
            )}
            Create Contract
          </button>
        </div>
      </div>
    )}
  </div>
  );
}
