import type { ReactNode } from "react";
import { Navigate, NavLink, Route, Routes, useNavigate } from "react-router-dom";
import { getToken, clearToken } from "./api";
import NodeFace from "./components/NodeFace";
import Login from "./pages/Login";
import Fleet from "./pages/Fleet";
import NodeDetail from "./pages/NodeDetail";
import Enroll from "./pages/Enroll";
import Commands from "./pages/Commands";

function Guard({ children }: { children: ReactNode }) {
  if (!getToken()) return <Navigate to="/login" replace />;
  return <>{children}</>;
}

function Shell() {
  const nav = useNavigate();
  return (
    <>
      <a href="#main" className="skip">
        Skip to content
      </a>
      <header className="topbar">
        <NavLink to="/" className="brand" end>
          <NodeFace size={40} />
          <span>Fleet Manager</span>
        </NavLink>
        <nav aria-label="Main">
          <NavLink to="/" end>
            Fleet
          </NavLink>
          <NavLink to="/enroll">Onboard</NavLink>
          <NavLink to="/commands">Commands</NavLink>
        </nav>
        <button
          className="btn secondary small"
          type="button"
          onClick={() => {
            clearToken();
            nav("/login");
          }}
        >
          Sign out
        </button>
      </header>
      <main id="main" className="main">
        <Routes>
          <Route path="/" element={<Fleet />} />
          <Route path="/nodes/:id" element={<NodeDetail />} />
          <Route path="/enroll" element={<Enroll />} />
          <Route path="/commands" element={<Commands />} />
        </Routes>
      </main>
    </>
  );
}

export default function App() {
  return (
    <Routes>
      <Route path="/login" element={<Login />} />
      <Route
        path="/*"
        element={
          <Guard>
            <Shell />
          </Guard>
        }
      />
    </Routes>
  );
}
