import { useEffect, useState } from 'react';

import type { IncidentListFilter } from '../../api';

/**
 * The state machine's terminal + non-terminal states. Hard-coded here
 * (rather than reading from the schema) because the dashboard's
 * audience picks states out of a fixed list.
 */
const ALL_STATES = [
  'pending',
  'triaged',
  'acknowledged',
  'remediating',
  'closed',
  'dismissed',
] as const;

const ALL_ECOSYSTEMS = ['npm', 'gomod', 'cargo', 'pypi', 'maven'] as const;

export type IncidentDashboardToolbarProps = {
  filter: IncidentListFilter;
  onChange: (next: IncidentListFilter) => void;
};

// Inline styles avoid pulling MUI Select/Stack into the bundle —
// they hit a Backstage webpack edge case that broke the SPA mount.
// See the component doc below for context.
const toolbarStyle: React.CSSProperties = {
  display: 'flex',
  flexDirection: 'column',
  gap: 12,
  padding: '12px 0',
};
const rowStyle: React.CSSProperties = {
  display: 'flex',
  flexWrap: 'wrap',
  gap: 16,
  alignItems: 'flex-end',
};
const labelStyle: React.CSSProperties = {
  display: 'flex',
  flexDirection: 'column',
  fontSize: 12,
  color: '#666',
  gap: 2,
};
const inputStyle: React.CSSProperties = {
  padding: '6px 8px',
  border: '1px solid #ccc',
  borderRadius: 4,
  fontSize: 14,
};
const chipGroupLabelStyle: React.CSSProperties = {
  fontSize: 12,
  color: '#666',
  display: 'block',
  marginBottom: 4,
};
const chipRowStyle: React.CSSProperties = {
  display: 'flex',
  flexWrap: 'wrap',
  gap: 6,
};
const chipBase: React.CSSProperties = {
  padding: '4px 10px',
  borderRadius: 16,
  fontSize: 12,
  border: '1px solid #ccc',
  background: '#fff',
  cursor: 'pointer',
};
const chipStyle: React.CSSProperties = { ...chipBase };
const chipActiveStyle: React.CSSProperties = {
  ...chipBase,
  background: '#1976d2',
  color: '#fff',
  borderColor: '#1976d2',
};

const FilterChip = ({
  label,
  active,
  onToggle,
}: {
  label: string;
  active: boolean;
  onToggle: () => void;
}) => (
  <button type="button" onClick={onToggle} style={active ? chipActiveStyle : chipStyle}>
    {label}
  </button>
);

/**
 * IncidentDashboardToolbar renders the filter UI for the dashboard.
 * Pure controlled component — the parent owns the filter state and
 * routes it through useSearchParams (URL is the single source of
 * truth, see IncidentDashboard).
 *
 * Implementation note: this component intentionally uses native HTML
 * form controls (`<input>`, `<button>`) rather than MUI primitives.
 * The first part-2 push tried MUI Select / Stack / Chip and hit a
 * `module-mui` runtime error in Backstage's production webpack
 * bundle that broke the SPA mount in the e2e suite. Native controls
 * sidestep the bundler edge case entirely; the visual difference is
 * acceptable for v0.2.0 — the v0.3.0 frontend hardening pass can
 * revisit with proper MUI integration once the Backstage + MUI 9
 * peer-dep story stabilises.
 *
 * The search input is debounced 300ms locally so a typist doesn't
 * spam the URL + the API.
 */
export const IncidentDashboardToolbar = ({
  filter,
  onChange,
}: IncidentDashboardToolbarProps) => {
  const [searchDraft, setSearchDraft] = useState(filter.search ?? '');

  // Sync local draft from external filter (URL navigation, back-button)
  // so the field stays consistent with the URL.
  useEffect(() => {
    setSearchDraft(filter.search ?? '');
  }, [filter.search]);

  // Debounce the search input → URL state. 300ms balances feel
  // (snappy enough for power users) against API churn.
  useEffect(() => {
    if ((filter.search ?? '') === searchDraft) return undefined;
    const handle = setTimeout(() => {
      onChange({ ...filter, search: searchDraft || undefined });
    }, 300);
    return () => clearTimeout(handle);
  }, [searchDraft, filter, onChange]);

  const states = filter.states ?? [];
  const ecosystems = filter.ecosystems ?? [];

  const toggleState = (s: string) => {
    const next = states.includes(s) ? states.filter(x => x !== s) : [...states, s];
    onChange({ ...filter, states: next.length ? next : undefined });
  };

  const toggleEcosystem = (e: string) => {
    const next = ecosystems.includes(e) ? ecosystems.filter(x => x !== e) : [...ecosystems, e];
    onChange({ ...filter, ecosystems: next.length ? next : undefined });
  };

  return (
    <div data-testid="incident-toolbar" style={toolbarStyle}>
      <div style={rowStyle}>
        <label style={labelStyle}>
          Search
          <input
            type="text"
            value={searchDraft}
            placeholder="incident id / ioc id / component ref"
            onChange={e => setSearchDraft(e.target.value)}
            data-testid="filter-search"
            style={{ ...inputStyle, minWidth: 280 }}
          />
        </label>
        <label style={labelStyle}>
          From
          <input
            type="datetime-local"
            value={filter.from ? toLocalInput(filter.from) : ''}
            onChange={e => onChange({ ...filter, from: fromLocalInput(e.target.value) })}
            data-testid="filter-from"
            style={inputStyle}
          />
        </label>
        <label style={labelStyle}>
          To
          <input
            type="datetime-local"
            value={filter.to ? toLocalInput(filter.to) : ''}
            onChange={e => onChange({ ...filter, to: fromLocalInput(e.target.value) })}
            data-testid="filter-to"
            style={inputStyle}
          />
        </label>
        <label style={labelStyle}>
          Owner
          <input
            type="text"
            value={filter.owner ?? ''}
            placeholder="team-platform"
            onChange={e => onChange({ ...filter, owner: e.target.value || undefined })}
            data-testid="filter-owner"
            style={{ ...inputStyle, minWidth: 180 }}
          />
        </label>
      </div>

      <div>
        <span style={chipGroupLabelStyle}>State</span>
        <div data-testid="filter-state" style={chipRowStyle}>
          {ALL_STATES.map(s => (
            <FilterChip key={s} label={s} active={states.includes(s)} onToggle={() => toggleState(s)} />
          ))}
        </div>
      </div>

      <div>
        <span style={chipGroupLabelStyle}>Ecosystem</span>
        <div data-testid="filter-ecosystem" style={chipRowStyle}>
          {ALL_ECOSYSTEMS.map(e => (
            <FilterChip key={e} label={e} active={ecosystems.includes(e)} onToggle={() => toggleEcosystem(e)} />
          ))}
        </div>
      </div>
    </div>
  );
};

// datetime-local inputs work with `YYYY-MM-DDTHH:mm`, no timezone.
// Engine wants RFC3339. Convert in both directions.
function toLocalInput(rfc3339: string): string {
  const d = new Date(rfc3339);
  const pad = (n: number) => String(n).padStart(2, '0');
  return (
    `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}` +
    `T${pad(d.getHours())}:${pad(d.getMinutes())}`
  );
}

function fromLocalInput(local: string): string | undefined {
  if (!local) return undefined;
  return new Date(local).toISOString();
}

/** filterFromSearchParams is the URL-state half of the toolbar
 *  contract. Lives next to the toolbar so the URL <-> filter shape
 *  stays in lock-step. The dashboard wires this on every render. */
export function filterFromSearchParams(params: URLSearchParams): IncidentListFilter {
  const states = params.getAll('state');
  const ecosystems = params.getAll('ecosystem');
  const out: IncidentListFilter = {};
  if (states.length) out.states = states;
  if (ecosystems.length) out.ecosystems = ecosystems;
  const from = params.get('from');
  if (from) out.from = from;
  const to = params.get('to');
  if (to) out.to = to;
  const search = params.get('search');
  if (search) out.search = search;
  const owner = params.get('owner');
  if (owner) out.owner = owner;
  return out;
}

/** filterToSearchParams writes the filter into a URLSearchParams the
 *  caller can pass to setSearchParams. Mirrors the engine's accepted
 *  query-param shape (multi-value: repeated key, single-value: set). */
export function filterToSearchParams(
  filter: IncidentListFilter,
  base: URLSearchParams,
): URLSearchParams {
  const out = new URLSearchParams(base);
  // Drop any keys we own so we never produce stale duplicates.
  ['state', 'ecosystem', 'from', 'to', 'search', 'owner'].forEach(k => out.delete(k));
  for (const s of filter.states ?? []) out.append('state', s);
  for (const e of filter.ecosystems ?? []) out.append('ecosystem', e);
  if (filter.from) out.set('from', filter.from);
  if (filter.to) out.set('to', filter.to);
  if (filter.search) out.set('search', filter.search);
  if (filter.owner) out.set('owner', filter.owner);
  return out;
}
