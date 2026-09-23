import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { useLocation, useNavigate } from 'react-router';
import AgentSidebar from './AgentSidebar';
import { getAgentContext, supportsAgent, type AgentMessage } from './agent-preview';
import { AgentStateContext } from './agent-context';
import { harnest, type HarnestStreamEvent } from '~/lib/harnest';
import { api } from '~/lib/api';
import { executeFrontendTool, type ContractDraft } from './client-tools';
import { applyProfileViewProposal, profileViewInstructions, type ProfileViewDesignerBridge } from '../profiles/view/profile-view';
import { updateToolActivity } from './tool-activity';
import { useAgentStatus } from './use-agent-status';

export default function AgentProvider({ children }: { children: ReactNode }) {
  const location = useLocation();
  const navigate = useNavigate();
  const signedInRoute = location.pathname.startsWith('/u/');
  const { status: agentStatus, checking: checkingAgentStatus, refresh: refreshAgentStatus } = useAgentStatus(signedInRoute);
  const enabled = signedInRoute && agentStatus.enabled;
  const isReady = enabled && agentStatus.status === 'ready';
  const isSupported = supportsAgent(location.pathname, location.search);
  const [isOpen, setIsOpen] = useState(false);
  const [isCompact, setIsCompact] = useState(() => window.matchMedia('(max-width: 1023px)').matches);
  const [includeContext, setIncludeContext] = useState(true);
  const [messages, setMessages] = useState<AgentMessage[]>([]);
  const [composer, setComposer] = useState('');
  const [isSending, setIsSending] = useState(false);
  const [error, setError] = useState('');
  const [contractDraft, setContractDraft] = useState<ContractDraft>({ source: '', revision: 0, open: false });
  const draftRef = useRef(contractDraft);
  const profileDesigner = useRef<ProfileViewDesignerBridge | null>(null);
  const registerProfileDesigner = useCallback((designer: ProfileViewDesignerBridge) => {
    profileDesigner.current = designer;
    return () => { if (profileDesigner.current === designer) profileDesigner.current = null; };
  }, []);
  const applyProfileProposal = (message: AgentMessage) => {
    applyProfileViewProposal(message.text, message.profileViewTarget, profileDesigner.current);
  };
  const sessionRef = useRef<string | null>(null);
  const runRef = useRef<AbortController | null>(null);
  const previousFocus = useRef<HTMLElement | null>(null);
  const context = useMemo(() => getAgentContext(location.pathname, location.search), [location.pathname, location.search]);
  const current = useRef({ context, includeContext, enabled });
  current.current = { context, includeContext, enabled };
  const visible = enabled && isSupported && isOpen;

  const writeDraft = useCallback((source: string, open = draftRef.current.open) => {
    const next = { source, revision: draftRef.current.revision + 1, open };
    draftRef.current = next;
    setContractDraft(next);
    return next;
  }, []);
  const editContractDraft = (source: string) => { writeDraft(source); };
  const setContractEditorOpen = (open: boolean) => {
    const next = { ...draftRef.current, open };
    draftRef.current = next;
    setContractDraft(next);
  };
  const stop = useCallback(() => {
    runRef.current?.abort();
    runRef.current = null;
    // A stopped model turn may be partially persisted; use a fresh server
    // session on the next request instead of resuming an unknown pending action.
    sessionRef.current = null;
    setIsSending(false);
    setMessages(previous => previous.map(message => ({ ...message,
      text: message.role === 'assistant' && !message.text ? 'Response stopped.' : message.text,
      tools: message.tools?.map(tool => tool.status === 'running' ? { ...tool, status: 'stopped' } : tool),
    })));
  }, []);
  useEffect(() => () => runRef.current?.abort(), []);

  const openAgent = useCallback(() => {
    if (!enabled || !isSupported) return;
    previousFocus.current = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    setIsOpen(true);
  }, [enabled, isSupported]);
  const closeAgent = useCallback(() => {
    setIsOpen(false);
    requestAnimationFrame(() => {
      const target = previousFocus.current?.isConnected && previousFocus.current.getClientRects().length
        ? previousFocus.current
        : Array.from(document.querySelectorAll<HTMLElement>('[data-agent-launcher]')).find(element => element.getClientRects().length);
      target?.focus();
    });
  }, []);

  useEffect(() => {
    const media = window.matchMedia('(max-width: 1023px)');
    const update = () => setIsCompact(media.matches);
    media.addEventListener('change', update);
    return () => media.removeEventListener('change', update);
  }, []);

  useEffect(() => {
    if (!enabled) {
      stop();
      setIsOpen(false);
      setMessages([]);
      setComposer('');
      setIncludeContext(true);
      setError('');
      if (!signedInRoute) writeDraft('', false);
    }
  }, [enabled, signedInRoute, stop, writeDraft]);

  useEffect(() => {
    if (!enabled || !isSupported) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 'j') {
        event.preventDefault();
        if (isOpen) closeAgent(); else openAgent();
      }
    };
    document.addEventListener('keydown', onKeyDown);
    return () => document.removeEventListener('keydown', onKeyDown);
  }, [enabled, isSupported, isOpen, openAgent, closeAgent]);

  useEffect(() => {
    if (!visible || !isCompact) return;
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    return () => {
      document.body.style.overflow = previousOverflow;
    };
  }, [visible, isCompact]);

  useEffect(() => {
    if (!isSupported) setIsOpen(false);
  }, [isSupported]);

  const sendMessage = async (text: string) => {
    const request = text.trim().slice(0, 4000);
    if (!request || runRef.current || !isReady || !isSupported) return;
    const controller = new AbortController();
    runRef.current = controller;
    setIsSending(true);
    setError('');
    const user: AgentMessage = { id: crypto.randomUUID(), role: 'user', text: request, context: includeContext ? context : undefined };
    const designer = includeContext ? profileDesigner.current : null;
    const profileViewTarget = designer ? { key: designer.key, revision: designer.revision } : undefined;
    const reply: AgentMessage = { profileViewTarget, role: 'assistant', text: '', id: crypto.randomUUID(), tools: [] };
    setMessages(previous => [...previous, user, reply]);
    setComposer('');
    const update = (change: (message: AgentMessage) => AgentMessage) => {
      if (controller.signal.aborted) return;
      setMessages(previous => previous.map(message => message.id === reply.id ? change(message) : message));
    };
    const toolStatus = (id: string, name: string, status: 'running' | 'completed' | 'failed', claimStreamedCall = false) => update(message => ({ ...message,
      tools: updateToolActivity(message.tools, { id, name, status }, claimStreamedCall),
    }));
    const onEvent = (event: HarnestStreamEvent) => {
      if (event.type === 'response.text.delta') update(message => ({ ...message, text: message.text + (event.delta ?? '') }));
      if (event.type === 'response.tool_call' && event.name) toolStatus(event.id ?? event.name, event.name, 'running');
      if (event.type === 'response.tool_result' && event.name) toolStatus(event.callId ?? event.name, event.name, 'completed');
      if (event.type === 'response.completed' && event.status === 'completed') update(message => ({ ...message, text: event.outputText || message.text,
        tools: message.tools?.map(tool => tool.status === 'running' ? { ...tool, status: 'completed' } : tool),
      }));
      if (event.type === 'error') throw new Error(typeof event.error === 'string' ? event.error : 'The agent could not complete this response.');
    };
    try {
      const session = sessionRef.current ?? (await harnest.createSession(request.slice(0, 70), controller.signal)).id;
      controller.signal.throwIfAborted();
      sessionRef.current = session;
      await harnest.streamResponse(designer ? `${request}\n\n${profileViewInstructions(designer)}` : request, session, onEvent, controller.signal, async call => {
        controller.signal.throwIfAborted();
        toolStatus(`client:${call.id}`, call.name, 'running', true);
        const output = await executeFrontendTool(call, {
          getContext: () => current.current.includeContext ? { ...current.current.context, contractDraft: draftRef.current, profileViewDraft: profileDesigner.current ? { definition: profileDesigner.current.definition, revision: profileDesigner.current.revision, authoringInstructions: profileViewInstructions(profileDesigner.current) } : undefined } : { pageContextEnabled: false },
          getDraft: () => draftRef.current,
          writeDraft: source => writeDraft(source, true),
          navigate: async path => {
            controller.signal.throwIfAborted();
            navigate(path);
            const deadline = Date.now() + 5000;
            while (current.current.context.path !== path) {
              controller.signal.throwIfAborted();
              if (Date.now() > deadline) throw new Error('Page navigation did not complete.');
              await new Promise(resolve => setTimeout(resolve, 20));
            }
          },
          previewDraft: async draft => {
            const result = await api.previewContract({ yaml: draft.source });
            controller.signal.throwIfAborted();
            if (draftRef.current.revision === draft.revision) {
              const next = { ...draftRef.current, preview: result };
              draftRef.current = next;
              setContractDraft(next);
            }
            return result;
          },
        }, controller.signal);
        toolStatus(`client:${call.id}`, call.name, output && typeof output === 'object' && 'ok' in output && !output.ok ? 'failed' : 'completed');
        return output;
      }, includeContext ? context : undefined);
    } catch (cause) {
      if (!controller.signal.aborted) {
        sessionRef.current = null;
        setError(cause instanceof Error ? cause.message : 'The agent could not complete this request.');
        update(message => ({ ...message, text: message.text || 'I couldn’t complete that request. You can try again.', tools: message.tools?.map(tool => tool.status === 'running' ? { ...tool, status: 'failed' } : tool) }));
      }
    } finally {
      if (runRef.current === controller) { runRef.current = null; setIsSending(false); }
    }
  };

  const newConversation = () => { stop(); setMessages([]); setComposer(''); setError(''); };

  return (
    <AgentStateContext.Provider value={{ isEnabled: enabled, agentStatus, checkingAgentStatus, refreshAgentStatus, registerProfileDesigner, applyProfileProposal, isOpen: visible, isSupported, isCompact, openAgent, closeAgent, context, includeContext, setIncludeContext, messages, composer, setComposer, sendMessage, newConversation, isSending, error, stop, contractDraft, editContractDraft, setContractEditorOpen }}>
      {children}
      {visible && <AgentSidebar isCompact={isCompact} />}
    </AgentStateContext.Provider>
  );
}
