import { rampartPlugin } from './plugin';
import { parseInterval } from './service/catalogSync';

describe('rampart-backend plugin', () => {
  it('is defined', () => {
    expect(rampartPlugin).toBeDefined();
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
