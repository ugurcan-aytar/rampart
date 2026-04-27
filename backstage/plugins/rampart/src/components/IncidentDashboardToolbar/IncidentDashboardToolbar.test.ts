import {
  filterFromSearchParams,
  filterToSearchParams,
} from './IncidentDashboardToolbar';

// The render-side of the toolbar (MUI Selects, debounced TextField,
// useSearchParams) needs jsdom + react-testing-library scaffolding
// that backstage-cli's bare jest config doesn't ship by default.
// We exercise the URL-state sync helpers directly — they're the
// load-bearing contract the dashboard relies on.

describe('filterFromSearchParams', () => {
  it('returns an empty filter when there are no params', () => {
    expect(filterFromSearchParams(new URLSearchParams())).toEqual({});
  });

  it('extracts multi-value state and ecosystem', () => {
    const p = new URLSearchParams('state=pending&state=triaged&ecosystem=npm');
    expect(filterFromSearchParams(p)).toEqual({
      states: ['pending', 'triaged'],
      ecosystems: ['npm'],
    });
  });

  it('extracts single-value scalars', () => {
    const p = new URLSearchParams('search=axios&owner=team-platform&from=2026-04-01T00:00:00Z');
    expect(filterFromSearchParams(p)).toEqual({
      search: 'axios',
      owner: 'team-platform',
      from: '2026-04-01T00:00:00Z',
    });
  });

  it('ignores keys outside the filter contract', () => {
    const p = new URLSearchParams('incident=inc-42&search=axios');
    expect(filterFromSearchParams(p)).toEqual({ search: 'axios' });
  });
});

describe('filterToSearchParams', () => {
  it('serialises an empty filter to a base-only param set', () => {
    const base = new URLSearchParams('incident=inc-42');
    const out = filterToSearchParams({}, base);
    // Drawer state preserved; no filter keys produced.
    expect(out.get('incident')).toBe('inc-42');
    expect(out.get('search')).toBeNull();
    expect(out.getAll('state')).toEqual([]);
  });

  it('appends multi-value state + ecosystem with repeated keys', () => {
    const out = filterToSearchParams(
      { states: ['pending', 'triaged'], ecosystems: ['npm'] },
      new URLSearchParams(),
    );
    expect(out.getAll('state')).toEqual(['pending', 'triaged']);
    expect(out.getAll('ecosystem')).toEqual(['npm']);
  });

  it('overwrites stale single-value scalars on subsequent calls', () => {
    const first = filterToSearchParams({ search: 'axios' }, new URLSearchParams());
    const second = filterToSearchParams({ search: 'lodash' }, first);
    expect(second.getAll('search')).toEqual(['lodash']);
  });

  it('drops filter keys absent from the new filter', () => {
    const first = filterToSearchParams({ states: ['pending'], search: 'axios' }, new URLSearchParams());
    const second = filterToSearchParams({}, first);
    expect(second.getAll('state')).toEqual([]);
    expect(second.get('search')).toBeNull();
  });

  it('round-trips through filterFromSearchParams', () => {
    const filter = {
      states: ['pending', 'remediating'],
      ecosystems: ['npm', 'gomod'],
      search: 'axios',
      owner: 'team-platform',
      from: '2026-04-01T00:00:00.000Z',
      to: '2026-04-30T23:59:59.000Z',
    };
    const params = filterToSearchParams(filter, new URLSearchParams());
    expect(filterFromSearchParams(params)).toEqual(filter);
  });
});
