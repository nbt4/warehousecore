import { useState, useEffect, useId, useLayoutEffect } from "react";
import { useBlocker } from "react-router-dom";
import { useAuth } from "../../contexts/useAuth";
export function useCatalogWriteAccess() {
  const { user } = useAuth();
  return (
    user?.roles?.some((role) =>
      ["admin", "warehouse_admin"].includes(role.name.toLowerCase()),
    ) ?? false
  );
}

export function useDebouncedValue<T>(value: T, delay = 250) {
  const [debounced, setDebounced] = useState(value);
  useEffect(() => {
    const timer = window.setTimeout(() => setDebounced(value), delay);
    return () => window.clearTimeout(timer);
  }, [value, delay]);
  return debounced;
}

type PendingEdit = { message: string; discard?: () => void };
const pendingEdits = new Map<string, PendingEdit>();

export function useUnsavedChanges(
  dirty: boolean,
  message: string,
  discard?: () => void,
) {
  const id = useId();
  useLayoutEffect(() => {
    if (dirty) pendingEdits.set(id, { message, discard });
    else pendingEdits.delete(id);
    return () => {
      pendingEdits.delete(id);
    };
  }, [id, dirty, message, discard]);
  // Saving can navigate before React commits the new clean form state.
  return () => {
    pendingEdits.delete(id);
  };
}

export function useUnsavedNavigationGuard() {
  const blocker = useBlocker(
    ({ currentLocation, nextLocation }) =>
      pendingEdits.size > 0 &&
      (currentLocation.pathname !== nextLocation.pathname ||
        currentLocation.search !== nextLocation.search),
  );
  useEffect(() => {
    if (blocker.state !== "blocked") return;
    const edits = [...pendingEdits.values()];
    if (!edits.length || window.confirm(edits[0].message)) {
      for (const edit of edits) edit.discard?.();
      blocker.proceed();
    } else blocker.reset();
  }, [blocker]);
  useEffect(() => {
    const unload = (event: BeforeUnloadEvent) => {
      if (!pendingEdits.size) return;
      event.preventDefault();
      event.returnValue = "";
    };
    window.addEventListener("beforeunload", unload);
    return () => window.removeEventListener("beforeunload", unload);
  }, []);
}
