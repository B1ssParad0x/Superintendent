import { NavLink } from 'react-router-dom'
import { motion } from 'framer-motion'
import { useAppAuth } from '../context/AuthProvider'

const navItems = [
  { to: '/dashboard', label: 'Dashboard' },
  { to: '/logs', label: 'Logs' },
  { to: '/nodes', label: 'Nodes' },
  { to: '/admin', label: 'Admin' },
  { to: '/about', label: 'About' },
]

export default function Navbar() {
  const { isAuthenticated, login, logout, roleLabel, devMode } = useAppAuth()
  return (
    <header className="sticky top-0 z-50 border-b border-zinc-800/80 bg-black/70 backdrop-blur">
      <div className="mx-auto flex max-w-7xl items-center justify-between px-4 py-3">
        <NavLink to="/" className="font-display text-xl text-white">
          Superintendent
        </NavLink>
        <nav className="flex items-center gap-2">
          {navItems.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              className={({ isActive }) =>
                `rounded-md px-3 py-1.5 text-sm transition ${isActive ? 'bg-crimson text-white' : 'text-zinc-300 hover:bg-zinc-900'}`
              }
            >
              {item.label}
            </NavLink>
          ))}
        </nav>
        <div className="flex items-center gap-3">
          <motion.span
            initial={{ opacity: 0.6 }}
            animate={{ opacity: [0.7, 1, 0.7] }}
            transition={{ duration: 3.5, repeat: Infinity }}
            className="rounded-full border border-crimson/50 px-2 py-1 text-xs uppercase tracking-wide text-zinc-100"
          >
            {devMode ? 'dev-admin' : roleLabel}
          </motion.span>
          {!isAuthenticated ? (
            <button className="rounded-md bg-crimson px-3 py-1.5 text-sm font-medium text-white" onClick={login}>
              Sign In
            </button>
          ) : (
            <button className="rounded-md border border-zinc-700 px-3 py-1.5 text-sm text-zinc-200" onClick={logout}>
              Logout
            </button>
          )}
        </div>
      </div>
    </header>
  )
}
