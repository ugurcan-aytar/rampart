import { rampartPlugin, resolveEngineConfig } from './plugin';
import { parseInterval } from './service/catalogSync';

type LoggerSpy = {
  info: jest.Mock<void, [string]>;
  warn: jest.Mock<void, [string]>;
};

function mkLogger(): LoggerSpy {
  return { info: jest.fn(), warn: jest.fn() };
}

function mkConfig(values: Record<string, string>) {
  return {
    getString(key: string): string {
      const v = values[key];
      if (v === undefined) throw new Error(`missing config: ${key}`);
      return v;
    },
    getOptionalString(key: string): string | undefined {
      return values[key];
    },
  };
}

describe('rampart-backend plugin', () => {
  it('is defined', () => {
    expect(rampartPlugin).toBeDefined();
  });
});

describe('resolveEngineConfig', () => {
  it('reads the v0.2.0 key first', () => {
    const logger = mkLogger();
    const cfg = mkConfig({
      'rampart.engine.baseUrl': 'http://engine:8080',
      'rampart.engine.authToken': 'token-xyz',
    });
    const resolved = resolveEngineConfig(cfg, logger);
    expect(resolved).toEqual({ baseUrl: 'http://engine:8080', authToken: 'token-xyz' });
    expect(logger.warn).not.toHaveBeenCalled();
  });

  it('falls back to the v0.1.x key and warns', () => {
    const logger = mkLogger();
    const cfg = mkConfig({ 'rampart.baseUrl': 'http://legacy:8080' });
    const resolved = resolveEngineConfig(cfg, logger);
    expect(resolved.baseUrl).toBe('http://legacy:8080');
    expect(resolved.authToken).toBeUndefined();
    expect(logger.warn).toHaveBeenCalledWith(expect.stringContaining('deprecated'));
  });

  it('throws when neither key is set', () => {
    const logger = mkLogger();
    const cfg = mkConfig({});
    expect(() => resolveEngineConfig(cfg, logger)).toThrow(/no engine URL configured/);
  });

  it('prefers v0.2.0 over v0.1.x when both are set (no warn)', () => {
    const logger = mkLogger();
    const cfg = mkConfig({
      'rampart.engine.baseUrl': 'http://new:8080',
      'rampart.baseUrl': 'http://old:8080',
    });
    const resolved = resolveEngineConfig(cfg, logger);
    expect(resolved.baseUrl).toBe('http://new:8080');
    expect(logger.warn).not.toHaveBeenCalled();
  });
});

describe('parseInterval', () => {
  it('parses the five supported suffixes', () => {
    expect(parseInterval('10ms')).toBe(10);
    expect(parseInterval('30s')).toBe(30_000);
    expect(parseInterval('5m')).toBe(300_000);
    expect(parseInterval('2h')).toBe(7_200_000);
    expect(parseInterval('1d')).toBe(86_400_000);
  });

  it('rejects garbage', () => {
    expect(() => parseInterval('5 mins')).toThrow(/invalid interval/);
    expect(() => parseInterval('')).toThrow(/invalid interval/);
    expect(() => parseInterval('1week')).toThrow(/invalid interval/);
  });
});
