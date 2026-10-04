import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter, Navigate, Route, Routes } from "react-router-dom";
import { AuthProvider, Shell, useAuth } from "./Shell";
import { AddressRecord, AddressSearch } from "./pages/Addresses";
import { AccessLog } from "./pages/AccessLog";
import { Fees } from "./pages/Fees";
import { LicenceList, LicenceRecord } from "./pages/Licences";
import { Overview } from "./pages/Overview";
import { PeopleSearch, PersonRecord } from "./pages/People";
import { Equality } from "./pages/Equality";
import { SignIn } from "./pages/SignIn";
import { VisitRecord, Worklist } from "./pages/Enforcement";
import "./styles.css";

function Routing() {
  const { session, ready } = useAuth();

  if (!ready) return <div className="booting">Loading…</div>;
  if (!session) return <SignIn />;

  if (session.role === "ENFORCEMENT") {
    return (
      <Shell>
        <Routes>
          <Route path="/worklist" element={<Worklist />} />
          <Route path="/worklist/:id" element={<VisitRecord />} />
          <Route path="*" element={<Navigate to="/worklist" replace />} />
        </Routes>
      </Shell>
    );
  }

  return (
    <Shell>
      <Routes>
        <Route path="/" element={<Overview />} />
        <Route path="/people" element={<PeopleSearch />} />
        <Route path="/people/:id" element={<PersonRecord />} />
        <Route path="/addresses" element={<AddressSearch />} />
        <Route path="/addresses/:id" element={<AddressRecord />} />
        <Route path="/licences" element={<LicenceList />} />
        <Route path="/licences/:id" element={<LicenceRecord />} />
        <Route path="/fees" element={<Fees />} />
        {session.role === "ADMIN" && <Route path="/reports" element={<Equality />} />}
        {session.role === "ADMIN" && <Route path="/access-log" element={<AccessLog />} />}
        <Route path="*" element={<Navigate to="/" replace />} />
      </Routes>
    </Shell>
  );
}

const root = document.getElementById("root");
if (!root) throw new Error("no #root element");

createRoot(root).render(
  <StrictMode>
    <BrowserRouter>
      <AuthProvider>
        <Routing />
      </AuthProvider>
    </BrowserRouter>
  </StrictMode>,
);
