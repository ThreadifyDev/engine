import { useCallback, useEffect, useRef, useState } from 'react';
import { agentPollInterval, disabledAgent, fetchAgentStatus, unavailableAgent } from './agent-status';

export function useAgentStatus(active: boolean) {
  const [status, setStatus] = useState(disabledAgent);
  const [checking, setChecking] = useState(false);
  const refreshRef = useRef<() => void>(() => {});
  const refresh = useCallback(() => refreshRef.current(), []);

  useEffect(() => {
    if (!active) { setStatus(disabledAgent); setChecking(false); return; }
    let current = disabledAgent;
    let disposed = false;
    let controller: AbortController | null = null;
    let timer: ReturnType<typeof setTimeout> | undefined;
    const check = async () => {
      if (disposed || controller) return;
      clearTimeout(timer);
      controller = new AbortController();
      const request = controller;
      const timeout = setTimeout(() => request.abort(), 10_000);
      setChecking(true);
      try { current = await fetchAgentStatus(request.signal); }
      catch { current = unavailableAgent(current); }
      finally {
        clearTimeout(timeout);
        controller = null;
        if (!disposed) {
          setStatus(current);
          setChecking(false);
          timer = setTimeout(check, agentPollInterval(current));
        }
      }
    };
    refreshRef.current = check;
    void check();
    window.addEventListener('focus', check);
    return () => {
      disposed = true;
      refreshRef.current = () => {};
      clearTimeout(timer);
      controller?.abort();
      window.removeEventListener('focus', check);
    };
  }, [active]);

  return { status: active ? status : disabledAgent, checking, refresh };
}
