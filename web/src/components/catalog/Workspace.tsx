import { useUnsavedNavigationGuard } from "./useCatalog";
import { useEffect, useRef, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { X } from "lucide-react";

export function WorkspaceHeader({
  title,
  description,
  children,
}: {
  title: string;
  description?: string;
  children?: ReactNode;
}) {
  return (
    <header className="flex flex-wrap items-end justify-between gap-4">
      <div className="min-w-0">
        <h1 className="suite-dashboard-title">{title}</h1>
        {description && (
          <p className="suite-dashboard-subtitle">{description}</p>
        )}
      </div>
      <div className="flex flex-wrap gap-2">{children}</div>
    </header>
  );
}

export function WorkspaceNotice({
  error,
  children,
  retry,
}: {
  error?: boolean;
  children: ReactNode;
  retry?: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div
      className="suite-card p-4 flex flex-wrap items-center justify-between gap-3"
      role={error ? "alert" : "status"}
      style={{ color: error ? "var(--color-error)" : "var(--text-secondary)" }}
    >
      <span>{children}</span>
      {retry && (
        <button className="suite-button" onClick={retry}>
          {t("catalog.retry")}
        </button>
      )}
    </div>
  );
}

export function WorkspaceLoading() {
  const { t } = useTranslation();
  return (
    <div className="suite-card p-5 space-y-4" role="status" aria-busy="true">
      <span className="sr-only">{t("catalog.loading")}</span>
      {[0, 1, 2].map((i) => (
        <div
          key={i}
          className="h-10 animate-pulse rounded-lg"
          style={{ background: "var(--surface-2)" }}
        />
      ))}
    </div>
  );
}

export function WorkspaceField({
  label,
  children,
  hint,
}: {
  label: string;
  children: ReactNode;
  hint?: string;
}) {
  return (
    <label
      className="flex min-w-0 flex-col gap-2 text-sm font-semibold"
      style={{ color: "var(--text-primary)" }}
    >
      <span>{label}</span>
      {children}
      {hint && (
        <span
          className="text-xs font-normal"
          style={{ color: "var(--text-secondary)" }}
        >
          {hint}
        </span>
      )}
    </label>
  );
}

// Native dialogs provide focus containment, Escape and focus restoration.
export function WorkspaceDialog({
  title,
  children,
  onClose,
}: {
  title: string;
  children: ReactNode;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const dialog = useRef<HTMLDialogElement>(null);
  const closeRef = useRef(onClose);
  useEffect(() => {
    closeRef.current = onClose;
  }, [onClose]);
  useEffect(() => {
    const node = dialog.current;
    const previous = document.activeElement as HTMLElement | null;
    node?.showModal();
    const cancel = (event: Event) => {
      event.preventDefault();
      closeRef.current();
    };
    node?.addEventListener("cancel", cancel);
    return () => {
      node?.removeEventListener("cancel", cancel);
      node?.close();
      previous?.focus();
    };
  }, []);
  return (
    <dialog
      ref={dialog}
      aria-label={title}
      className="suite-card m-auto w-[calc(100%-2rem)] max-w-4xl max-h-[90dvh] p-0"
      style={{ color: "var(--text-primary)", boxShadow: "var(--shadow-modal)" }}
    >
      <div className="flex items-center justify-between gap-4 p-5">
        <h2 className="text-xl font-bold">{title}</h2>
        <button
          className="suite-button"
          onClick={onClose}
          aria-label={t("catalog.close")}
        >
          <X size={18} />
        </button>
      </div>
      <div className="overflow-y-auto px-5 pb-5">{children}</div>
    </dialog>
  );
}

export function UnsavedNavigationGuard() {
  useUnsavedNavigationGuard();
  return null;
}
