"use client";

import { useState } from "react";
import { api } from "@/lib/account";

// What can be done to a session, as a sheet: three dots are always there, since
// a phone has no hover. Deleting takes no confirmation here, because it comes
// with an Undo; clearing has no Undo, so it asks once.

export type MenuSession = { id: string; name: string; pinned: boolean; archived: boolean };

export default function SessionMenu({ session, onClose, onChanged, onDelete, onCleared, onSelect }: {
  session: MenuSession;
  onClose: () => void;
  /** After a rename, pin or archive: read it again. */
  onChanged: () => void;
  onDelete: () => void;
  onCleared?: () => void;
  onSelect?: () => void;
}) {
  const [naming, setNaming] = useState(false);
  const [name, setName] = useState(session.name);
  const [clearing, setClearing] = useState(false);
  const [error, setError] = useState("");
  const path = `/app/sessions/${encodeURIComponent(session.id)}`;
  const change = async (body: object) => {
    try {
      await api(path, { method: "PATCH", body: JSON.stringify(body) });
      onChanged();
      onClose();
    } catch (e) { setError((e as Error).message); }
  };
  return (
    <div className="sheet-back" onClick={onClose}>
      <div className="sheet" role="dialog" aria-label={`Session: ${session.name}`} onClick={(e) => e.stopPropagation()}>
        {naming ? (
          <form onSubmit={(e) => { e.preventDefault(); change({ title: name }); }}>
            <label htmlFor="session-name" className="sheet-label">Name</label>
            <input id="session-name" className="sheet-input" autoFocus value={name} maxLength={120} onChange={(e) => setName(e.target.value)} />
            <button type="submit" className="btn-act sheet-wide">Save</button>
          </form>
        ) : clearing ? (
          <>
            <p className="sheet-note">Clear every message from this chat? This can&apos;t be undone. The agent&apos;s own history on your laptop stays.</p>
            <button type="button" className="sheet-item danger" onClick={async () => {
              try {
                await api(`${path}/clear`, { method: "POST", body: "{}" });
                onCleared?.();
                onClose();
              } catch (e) { setError((e as Error).message); }
            }}>Clear chat</button>
            <button type="button" className="sheet-item" onClick={() => setClearing(false)}>Keep messages</button>
          </>
        ) : (
          <>
            <strong className="sheet-title">{session.name}</strong>
            <button type="button" className="sheet-item" onClick={() => setNaming(true)}>Rename</button>
            <button type="button" className="sheet-item" onClick={() => change({ pinned: !session.pinned })}>{session.pinned ? "Unpin" : "Pin"}</button>
            <button type="button" className="sheet-item" onClick={() => change({ archived: !session.archived })}>{session.archived ? "Unarchive" : "Archive"}</button>
            {onSelect && <button type="button" className="sheet-item" onClick={() => { onSelect(); onClose(); }}>Select several</button>}
            {onCleared && <button type="button" className="sheet-item danger" onClick={() => setClearing(true)}>Clear chat</button>}
            <button type="button" className="sheet-item danger" onClick={() => { onDelete(); onClose(); }}>Delete</button>
            <p className="sheet-note">Deleting removes it from trackline, not from your laptop.</p>
          </>
        )}
        {error && <p className="composer-note error">{error}</p>}
      </div>
    </div>
  );
}

/** A question with one answer that matters, in place of the browser's own box. */
export function ConfirmSheet({ title, action, onConfirm, onClose }: { title: string; action: string; onConfirm: () => void; onClose: () => void }) {
  return (
    <div className="sheet-back" onClick={onClose}>
      <div className="sheet" role="dialog" aria-label={title} onClick={(e) => e.stopPropagation()}>
        <p className="sheet-note">{title}</p>
        <button type="button" className="sheet-item danger" onClick={() => { onConfirm(); onClose(); }}>{action}</button>
        <button type="button" className="sheet-item" onClick={onClose}>Cancel</button>
      </div>
    </div>
  );
}

/** A name to give something, in place of the browser's own prompt. */
export function NameSheet({ label, value, onSave, onClose }: { label: string; value: string; onSave: (name: string) => void; onClose: () => void }) {
  const [name, setName] = useState(value);
  return (
    <div className="sheet-back" onClick={onClose}>
      <form className="sheet" role="dialog" aria-label={label} onClick={(e) => e.stopPropagation()} onSubmit={(e) => { e.preventDefault(); onSave(name.trim() || value); onClose(); }}>
        <label htmlFor="sheet-name" className="sheet-label">{label}</label>
        <input id="sheet-name" className="sheet-input" autoFocus value={name} maxLength={60} onChange={(e) => setName(e.target.value)} />
        <button type="submit" className="btn-act sheet-wide">Continue</button>
      </form>
    </div>
  );
}
