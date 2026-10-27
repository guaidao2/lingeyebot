import { X } from "lucide-react";
import { useEffect, useRef } from "react";
import type { ReactNode } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";

export function Badge({ tone = "muted", children }: { tone?: string; children: ReactNode }) {
  return <span className={`badge badge-${tone}`}>{children}</span>;
}

export function Spinner({ label }: { label?: string }) {
  return (
    <span className="spinner-wrap">
      <span className="spinner" />
      {label ? <span>{label}</span> : null}
    </span>
  );
}

export function Modal({
  title,
  subtitle,
  onClose,
  children,
  wide,
}: {
  title: string;
  subtitle?: string;
  onClose: () => void;
  children: ReactNode;
  wide?: boolean;
}) {
  const dialogRef = useRef<HTMLDivElement>(null);
  const onCloseRef = useRef(onClose);
  useEffect(() => {
    onCloseRef.current = onClose;
  }, [onClose]);
  useEffect(() => {
    // Focus the dialog exactly once on mount. Depending on `onClose` here
    // re-runs this effect whenever the parent re-renders and steals focus
    // from an input the user is typing in.
    const previous = document.activeElement instanceof HTMLElement ? document.activeElement : null;
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") onCloseRef.current();
    };
    document.addEventListener("keydown", onKey);
    dialogRef.current?.focus();
    return () => {
      document.removeEventListener("keydown", onKey);
      if (previous && previous.isConnected) previous.focus();
    };
  }, []);
  return (
    <div className="modal-backdrop">
      <div ref={dialogRef} tabIndex={-1} className={`modal ${wide ? "modal-wide" : ""}`} role="dialog" aria-modal="true" aria-labelledby="modal-title">
        <header className="modal-head">
          <div>
            <h2 id="modal-title">{title}</h2>
            {subtitle ? <p>{subtitle}</p> : null}
          </div>
          <button className="icon-btn" onClick={onClose} aria-label="关闭">
            <X size={18} />
          </button>
        </header>
        <div className="modal-body">{children}</div>
      </div>
    </div>
  );
}

export function Markdown({ content }: { content: string }) {
  if (!content.trim()) return <p className="muted">（空）</p>;
  return (
    <div className="markdown">
      <ReactMarkdown remarkPlugins={[remarkGfm]}>{content}</ReactMarkdown>
    </div>
  );
}

export function Empty({
  icon,
  title,
  description,
  action,
}: {
  icon?: ReactNode;
  title: string;
  description?: string;
  action?: ReactNode;
}) {
  return (
    <div className="empty">
      {icon ? <div className="empty-icon">{icon}</div> : null}
      <h3>{title}</h3>
      {description ? <p>{description}</p> : null}
      {action}
    </div>
  );
}

export function Collapsible({
  title,
  meta,
  children,
  defaultOpen = false,
}: {
  title: ReactNode;
  meta?: ReactNode;
  children: ReactNode;
  defaultOpen?: boolean;
}) {
  return (
    <details className="collapsible" open={defaultOpen}>
      <summary>
        <span className="collapsible-title">{title}</span>
        {meta ? <span className="collapsible-meta">{meta}</span> : null}
      </summary>
      <div className="collapsible-body">{children}</div>
    </details>
  );
}
