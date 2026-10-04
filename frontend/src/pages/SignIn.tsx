import { useState, type FormEvent } from "react";
import { api, asApiError, type ApiError } from "../api";
import { LicenceFooter, useAuth } from "../Shell";
import { Banner, ErrorSummary, Field, usePageTitle } from "../ui";

export function SignIn() {
  const { setSession, expired } = useAuth();
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<ApiError | null>(null);
  const [busy, setBusy] = useState(false);
  usePageTitle("Sign in");

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    setBusy(true);
    setError(null);
    try {
      setSession(await api.signIn(username, password));
    } catch (err) {
      setError(asApiError(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="signin-page">
      <div className="signin-card">
        <p className="wordmark wordmark-large">Aerial</p>
        <p className="signin-service">Licence administration</p>

        {expired && !error && (
          <Banner tone="warning" title="Your session has ended">
            Sign in again to carry on. Nothing you had already saved was lost.
          </Banner>
        )}
        <ErrorSummary error={error} />

        <form onSubmit={submit} noValidate>
          <Field label="Username">
            <input
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              autoComplete="username"
              autoFocus
              required
            />
          </Field>
          <Field label="Password">
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete="current-password"
              required
            />
          </Field>
          <button className="button" type="submit" disabled={busy}>
            {busy ? "Signing in…" : "Sign in"}
          </button>
        </form>

        {/*
          Only in a development build. A production bundle must not ship a
          list of usernames and a default password on its front door.
        */}
        {import.meta.env.DEV && (
          <div className="signin-note">
            <p className="signin-note-title">Development accounts</p>
            <ul>
              <li>
                <code>a.reyes</code> — customer services
              </li>
              <li>
                <code>j.okafor</code> — visiting officer
              </li>
              <li>
                <code>m.whitlock</code> — administrator
              </li>
            </ul>
            <p>
              Password <code>aerial-development</code>, unless AERIAL_DEV_PASSWORD is set.
            </p>
          </div>
        )}
      </div>
      <LicenceFooter />
    </div>
  );
}
