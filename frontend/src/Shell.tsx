import { createContext, useContext, useEffect, useState, type ReactNode } from "react";
import { NavLink, useNavigate } from "react-router-dom";
import { api, ApiError, setUnauthorizedHandler, type LicenceNotice, type Session } from "./api";
import { IconChip, type Hue } from "./icons";

interface AuthValue {
  session: Session | null;
  ready: boolean;
  /** Set when the session ended on its own rather than by signing out. */
  expired: boolean;
  setSession: (s: Session | null) => void;
}

const AuthContext = createContext<AuthValue>({
  session: null,
  ready: false,
  expired: false,
  setSession: () => {},
});

export const useAuth = () => useContext(AuthContext);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [session, setSession] = useState<Session | null>(null);
  const [ready, setReady] = useState(false);
  const [expired, setExpired] = useState(false);

  useEffect(() => {
    // A twelve-hour session will expire mid-shift. When any request comes
    // back unauthorised, drop to the sign-in screen and say why, rather than
    // leaving somebody clicking at a page that will never load again.
    setUnauthorizedHandler(() => {
      setSession((current) => {
        if (current) setExpired(true);
        return null;
      });
    });
    return () => setUnauthorizedHandler(() => {});
  }, []);

  useEffect(() => {
    api
      .whoAmI()
      .then((s) => {
        setSession(s);
        setExpired(false);
      })
      .catch((e: unknown) => {
        if (!(e instanceof ApiError) || e.status !== 401) console.error(e);
        setSession(null);
      })
      .finally(() => setReady(true));
  }, []);

  const adopt = (s: Session | null) => {
    setSession(s);
    if (s) setExpired(false);
  };

  return (
    <AuthContext.Provider value={{ session, ready, expired, setSession: adopt }}>
      {children}
    </AuthContext.Provider>
  );
}

interface NavItem {
  to: string;
  label: string;
  icon: string;
  hue: Hue;
  end?: boolean;
}

const STAFF_NAV: NavItem[] = [
  { to: "/", label: "Overview", icon: "overview", hue: "sky", end: true },
  { to: "/people", label: "People", icon: "people", hue: "mint" },
  { to: "/addresses", label: "Addresses", icon: "home", hue: "peach" },
  { to: "/licences", label: "Licences", icon: "licence", hue: "lilac" },
  { to: "/fees", label: "Fees and payments", icon: "money", hue: "teal" },
];

const ENFORCEMENT_NAV: NavItem[] = [
  { to: "/worklist", label: "Visit list", icon: "pin", hue: "peach", end: true },
];

export function Shell({ children }: { children: ReactNode }) {
  const { session, setSession } = useAuth();
  const navigate = useNavigate();

  if (!session) return <>{children}</>;

  const nav: NavItem[] =
    session.role === "ENFORCEMENT"
      ? ENFORCEMENT_NAV
      : session.role === "ADMIN"
        ? [
            ...STAFF_NAV,
            { to: "/reports", label: "Reports", icon: "chart", hue: "rose" },
            { to: "/access-log", label: "Access log", icon: "shield", hue: "sand" },
          ]
        : STAFF_NAV;

  const signOut = async () => {
    await api.signOut().catch(() => undefined);
    setSession(null);
    navigate("/");
  };

  return (
    <div className="shell">
      <a className="skip-link" href="#main">
        Skip to the main content
      </a>

      <header className="masthead">
        <div className="masthead-inner">
          <span className="wordmark">Aerial</span>
          <span className="masthead-sep" aria-hidden="true" />
          <span className="masthead-service">Licence administration</span>
          <div className="masthead-user">
            <span className="user-name">{session.displayName}</span>
            <span className="user-role">{roleWords(session.role)}</span>
            <button className="link-button" onClick={signOut}>
              Sign out
            </button>
          </div>
        </div>
      </header>

      <div className="body">
        <nav className="sidebar" aria-label="Sections">
          <ul>
            {nav.map((item) => (
              <li key={item.to}>
                <NavLink to={item.to} end={item.end ?? false}>
                  <IconChip name={item.icon} hue={item.hue} />
                  {item.label}
                </NavLink>
              </li>
            ))}
          </ul>
        </nav>
        <main className="content" id="main" tabIndex={-1}>
          {children}
        </main>
      </div>

      <LicenceFooter />
    </div>
  );
}

/**
 * The terms the running service is licensed under, read from the service
 * itself. The licence requires its notices to be kept and its terms described
 * accurately, and a console that never says so is a console whose operator
 * does not know.
 */
export function LicenceFooter() {
  const [notice, setNotice] = useState<LicenceNotice | null>(null);

  useEffect(() => {
    api.licenceNotice().then(setNotice).catch(() => setNotice(null));
  }, []);

  if (!notice) return null;

  return (
    <footer className="licence-footer">
      <p>
        <strong>
          {notice.service} {notice.version}
        </strong>{" "}
        · {notice.copyright} · {notice.licence}
      </p>
      <p className="licence-footer-terms">{notice.summary}</p>
    </footer>
  );
}

export function roleWords(role: string): string {
  switch (role) {
    case "SERVICE":
      return "Customer services";
    case "ENFORCEMENT":
      return "Visiting officer";
    case "ADMIN":
      return "Administrator";
    case "SUBJECT":
      return "Licence holder";
    default:
      return role;
  }
}
