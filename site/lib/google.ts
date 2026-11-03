import { GOOGLE_CLIENT_ID } from "./account";

// Google's own sign-in button, from Google's script. The ID token it returns
// carries the nonce it was given, which is how a token names the one message,
// or the one confirmation, it was made for.

type Gis = {
  accounts: { id: {
    initialize: (o: { client_id: string; nonce?: string; auto_select?: boolean; callback: (r: { credential: string }) => void }) => void;
    renderButton: (el: HTMLElement, o: Record<string, unknown>) => void;
  } };
};

let loading: Promise<Gis> | null = null;

function load(): Promise<Gis> {
  loading ??= new Promise((resolve, reject) => {
    const ready = (window as unknown as { google?: Gis }).google;
    if (ready) return resolve(ready);
    const s = document.createElement("script");
    s.src = "https://accounts.google.com/gsi/client";
    s.async = true;
    s.onload = () => {
      const g = (window as unknown as { google?: Gis }).google;
      if (g) resolve(g); else reject(new Error("Google could not be reached."));
    };
    s.onerror = () => { loading = null; reject(new Error("Google could not be reached.")); };
    document.head.appendChild(s);
  });
  return loading;
}

/** Puts Google's button in el; onToken gets the ID token made for nonce. */
export async function googleButton(el: HTMLElement, nonce: string, onToken: (token: string) => void) {
  const g = await load();
  g.accounts.id.initialize({ client_id: GOOGLE_CLIENT_ID, nonce, auto_select: true, callback: ({ credential }) => onToken(credential) });
  g.accounts.id.renderButton(el, {
    theme: "outline", size: "large", width: Math.min(400, el.offsetWidth || 320), text: "continue_with", locale: "en", logo_alignment: "center",
  });
}
