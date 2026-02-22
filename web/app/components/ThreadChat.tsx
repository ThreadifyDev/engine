import { useState, useRef, useEffect } from 'react';
import { Send, Bot, User, Loader2, Trash2, ChevronDown, Search } from 'lucide-react';
import { api } from '~/lib/api';

interface Message {
  id: string;
  role: 'user' | 'assistant' | 'tool';
  content: string;
  toolCalls?: any[];
  toolCallId?: string;
  timestamp: Date;
}

interface Conversation {
  id: string;
  title: string;
  created_at: string;
  updated_at: string;
}

export default function ThreadChat() {
  const [messages, setMessages] = useState<Message[]>([]);
  const [input, setInput] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const [conversationId, setConversationId] = useState<string | null>(null);
  const [conversations, setConversations] = useState<Conversation[]>([]);
  const [isDropdownOpen, setIsDropdownOpen] = useState(false);
  const [searchQuery, setSearchQuery] = useState('');
  const messagesEndRef = useRef<HTMLDivElement>(null);
  const dropdownRef = useRef<HTMLDivElement>(null);

  // Load conversations on mount
  useEffect(() => {
    loadConversations();
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
      console.error('Failed to load conversations:', error);
    }
  };

  const loadConversation = async (convId: string) => {
    try {
      const response = await api.getChatMessageHistory(convId);
      const msgs = response.messages.map((msg: any) => ({
        id: msg.id,
        role: msg.role,
        content: msg.content,
        timestamp: new Date(msg.created_at),
      }));
      setMessages(msgs);
      setConversationId(convId);
      setIsDropdownOpen(false);
      setSearchQuery('');
    } catch (error) {
      console.error('Failed to load conversation:', error);
    }
  };

  const startNewConversation = () => {
    setMessages([]);
    setConversationId(null);
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
      console.error('Failed to delete conversation:', error);
      alert('Failed to delete conversation');
    }
  };

  const scrollToBottom = () => {
    messagesEndRef.current?.scrollIntoView({ behavior: 'smooth' });
  };

  useEffect(() => {
    scrollToBottom();
  }, [messages]);

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
        throw new Error('Failed to get response');
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
                }
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
      console.error('Chat error:', error);
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

  return (
    <div className="flex flex-col h-full">
      {/* Header with Custom Conversation Dropdown */}
      <div className="border-b border-gray-200 px-4 py-3 bg-white">
        <div className="flex items-center gap-3">
          <div className="flex items-center gap-2">
            <Bot className="w-5 h-5 text-gray-900" />
            <h2 className="text-sm font-semibold text-gray-900">AI Thread Analyzer</h2>
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
            <p className="text-sm text-gray-500">Start a conversation to analyze your threads</p>
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
              <div className="text-sm whitespace-pre-wrap break-words">
                {message.content}
              </div>
              <p className="text-xs opacity-60 mt-1">
                {message.timestamp.toLocaleTimeString()}
              </p>
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
      <form onSubmit={handleSubmit} className="border-t border-gray-200 p-4">
        <div className="flex gap-2">
          <input
            type="text"
            value={input}
            onChange={(e) => setInput(e.target.value)}
            placeholder="Ask about your threads..."
            className="flex-1 px-3 py-2 text-sm border border-gray-300 rounded-md focus:outline-none focus:ring-2 focus:ring-gray-900"
            disabled={isLoading}
          />
          <button
            type="submit"
            disabled={!input.trim() || isLoading}
            className="px-4 py-2 bg-gray-900 text-white rounded-md hover:bg-gray-800 disabled:opacity-50 disabled:cursor-not-allowed transition-colors flex items-center gap-2"
          >
            <Send className="w-4 h-4" />
          </button>
        </div>
      </form>
    </div>
  );
}
