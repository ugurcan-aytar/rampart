import { rampartPlugin, rampartRouteRef, rampartIncidentRouteRef } from './plugin';
import { rampartApiRef } from './api';

describe('rampartPlugin', () => {
  it('exposes a plugin with the expected id', () => {
    expect(rampartPlugin).toBeDefined();
    // The plugin id is what Backstage keys api factories + route refs by.
    expect(rampartPlugin.getId()).toBe('rampart');
  });

  it('exposes two route refs', () => {
    expect(rampartRouteRef).toBeDefined();
    expect(rampartIncidentRouteRef).toBeDefined();
  });

  it('exposes an api ref for the RampartClient', () => {
    expect(rampartApiRef.id).toBe('plugin.rampart.api');
  });
});
