import { useEffect, useMemo, useState } from 'react';
import Box from '@mui/material/Box';
import Checkbox from '@mui/material/Checkbox';
import Chip from '@mui/material/Chip';
import FormControl from '@mui/material/FormControl';
import InputLabel from '@mui/material/InputLabel';
import ListItemText from '@mui/material/ListItemText';
import MenuItem from '@mui/material/MenuItem';
import OutlinedInput from '@mui/material/OutlinedInput';
import Select from '@mui/material/Select';
import type { SelectChangeEvent } from '@mui/material/Select';
import TextField from '@mui/material/TextField';

import type { IncidentListFilter } from '../../api';

/**
 * The state machine's terminal + non-terminal states. Hard-coded here
 * (rather than reading from the schema) because the dashboard's
 * audience picks states out of a fixed list — the TS gen-types are
 * union literals and don't iterate cleanly into a multi-select.
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

/**
 * IncidentDashboardToolbar renders the filter UI for the dashboard.
 * It's a pure controlled component — the parent owns the filter state
 * and routes it through useSearchParams (URL is the single source of
 * truth, see IncidentDashboard).
 *
 * The search input is debounced 300ms locally so a typist doesn't
 * spam the URL + the API; multi-select dropdowns commit immediately.
 */
export const IncidentDashboardToolbar = ({
  filter,
  onChange,
}: IncidentDashboardToolbarProps) => {
  const [searchDraft, setSearchDraft] = useState(filter.search ?? '');

  // Sync local draft from external filter (e.g. URL navigation,
  // back-button) so the field stays consistent with the URL.
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

  const states = useMemo(() => filter.states ?? [], [filter.states]);
  const ecosystems = useMemo(() => filter.ecosystems ?? [], [filter.ecosystems]);

  const handleStateChange = (event: SelectChangeEvent<string[]>) => {
    const value = event.target.value;
    const next = typeof value === 'string' ? value.split(',') : value;
    onChange({ ...filter, states: next.length ? next : undefined });
  };

  const handleEcosystemChange = (event: SelectChangeEvent<string[]>) => {
    const value = event.target.value;
    const next = typeof value === 'string' ? value.split(',') : value;
    onChange({ ...filter, ecosystems: next.length ? next : undefined });
  };

  return (
    <Box
      data-testid="incident-toolbar"
      sx={{
        display: 'flex',
        flexWrap: 'wrap',
        gap: 2,
        alignItems: 'center',
        py: 2,
      }}
    >
      <TextField
        size="small"
        label="Search"
        placeholder="incident id / ioc id / component ref"
        value={searchDraft}
        onChange={e => setSearchDraft(e.target.value)}
        sx={{ minWidth: 280 }}
        slotProps={{ htmlInput: { 'data-testid': 'filter-search' } }}
      />

      <FormControl size="small" sx={{ minWidth: 200 }}>
        <InputLabel>State</InputLabel>
        <Select
          multiple
          value={states}
          onChange={handleStateChange}
          input={<OutlinedInput label="State" />}
          renderValue={selected => (
            <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.5 }}>
              {selected.map(s => (
                <Chip key={s} label={s} size="small" />
              ))}
            </Box>
          )}
          inputProps={{ 'data-testid': 'filter-state' }}
        >
          {ALL_STATES.map(s => (
            <MenuItem key={s} value={s}>
              <Checkbox checked={states.includes(s)} />
              <ListItemText primary={s} />
            </MenuItem>
          ))}
        </Select>
      </FormControl>

      <FormControl size="small" sx={{ minWidth: 200 }}>
        <InputLabel>Ecosystem</InputLabel>
        <Select
          multiple
          value={ecosystems}
          onChange={handleEcosystemChange}
          input={<OutlinedInput label="Ecosystem" />}
          renderValue={selected => (
            <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.5 }}>
              {selected.map(s => (
                <Chip key={s} label={s} size="small" />
              ))}
            </Box>
          )}
          inputProps={{ 'data-testid': 'filter-ecosystem' }}
        >
          {ALL_ECOSYSTEMS.map(e => (
            <MenuItem key={e} value={e}>
              <Checkbox checked={ecosystems.includes(e)} />
              <ListItemText primary={e} />
            </MenuItem>
          ))}
        </Select>
      </FormControl>

      <TextField
        size="small"
        label="From"
        type="datetime-local"
        value={filter.from ? toLocalInput(filter.from) : ''}
        onChange={e => onChange({ ...filter, from: fromLocalInput(e.target.value) })}
        slotProps={{
          inputLabel: { shrink: true },
          htmlInput: { 'data-testid': 'filter-from' },
        }}
      />

      <TextField
        size="small"
        label="To"
        type="datetime-local"
        value={filter.to ? toLocalInput(filter.to) : ''}
        onChange={e => onChange({ ...filter, to: fromLocalInput(e.target.value) })}
        slotProps={{
          inputLabel: { shrink: true },
          htmlInput: { 'data-testid': 'filter-to' },
        }}
      />

      <TextField
        size="small"
        label="Owner"
        placeholder="team-platform"
        value={filter.owner ?? ''}
        onChange={e => onChange({ ...filter, owner: e.target.value || undefined })}
        sx={{ minWidth: 180 }}
        slotProps={{ htmlInput: { 'data-testid': 'filter-owner' } }}
      />
    </Box>
  );
};

// datetime-local inputs work with `YYYY-MM-DDTHH:mm`, no timezone.
// Engine wants RFC3339. Convert in both directions.
function toLocalInput(rfc3339: string): string {
  // Strip seconds + Z; the input doesn't accept the trailing Z form.
  const d = new Date(rfc3339);
  const pad = (n: number) => String(n).padStart(2, '0');
  return (
    `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}` +
    `T${pad(d.getHours())}:${pad(d.getMinutes())}`
  );
}

function fromLocalInput(local: string): string | undefined {
  if (!local) return undefined;
  // datetime-local inputs are local-time; serialise back to UTC RFC3339.
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
