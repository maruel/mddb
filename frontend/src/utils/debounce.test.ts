import { describe, it, expect, vi } from 'vitest';
import { debounce } from './debounce';

describe('debounce', () => {
  it('calls the function after the delay', async () => {
    vi.useFakeTimers();
    const fn = vi.fn();
    const debounced = debounce(fn, 100);

    debounced();
    expect(fn).not.toHaveBeenCalled();

    vi.advanceTimersByTime(100);
    expect(fn).toHaveBeenCalledOnce();

    vi.useRealTimers();
  });

  it('resets the delay on subsequent calls', async () => {
    vi.useFakeTimers();
    const fn = vi.fn();
    const debounced = debounce(fn, 100);

    debounced();
    vi.advanceTimersByTime(50);
    debounced();
    vi.advanceTimersByTime(50);
    expect(fn).not.toHaveBeenCalled();

    vi.advanceTimersByTime(50);
    expect(fn).toHaveBeenCalledOnce();

    vi.useRealTimers();
  });

  it('passes arguments to the debounced function', () => {
    vi.useFakeTimers();
    const fn = vi.fn();
    const debounced = debounce(fn, 100);

    debounced('arg1', 'arg2', 123);
    vi.advanceTimersByTime(100);

    expect(fn).toHaveBeenCalledWith('arg1', 'arg2', 123);
    vi.useRealTimers();
  });

  it('uses the last arguments when called multiple times', () => {
    vi.useFakeTimers();
    const fn = vi.fn();
    const debounced = debounce(fn, 100);

    debounced('first');
    vi.advanceTimersByTime(50);
    debounced('second');
    vi.advanceTimersByTime(50);
    debounced('third');
    vi.advanceTimersByTime(100);

    expect(fn).toHaveBeenCalledOnce();
    expect(fn).toHaveBeenCalledWith('third');
    vi.useRealTimers();
  });

  it('can be called again after the delay has passed', () => {
    vi.useFakeTimers();
    const fn = vi.fn();
    const debounced = debounce(fn, 100);

    debounced('first');
    vi.advanceTimersByTime(100);
    expect(fn).toHaveBeenCalledTimes(1);
    expect(fn).toHaveBeenLastCalledWith('first');

    debounced('second');
    vi.advanceTimersByTime(100);
    expect(fn).toHaveBeenCalledTimes(2);
    expect(fn).toHaveBeenLastCalledWith('second');

    vi.useRealTimers();
  });

  it('handles zero delay', () => {
    vi.useFakeTimers();
    const fn = vi.fn();
    const debounced = debounce(fn, 0);

    debounced();
    expect(fn).not.toHaveBeenCalled();

    vi.advanceTimersByTime(0);
    expect(fn).toHaveBeenCalledOnce();

    vi.useRealTimers();
  });

  it('handles rapid consecutive calls', () => {
    vi.useFakeTimers();
    const fn = vi.fn();
    const debounced = debounce(fn, 100);

    for (let i = 0; i < 100; i++) {
      debounced(i);
      vi.advanceTimersByTime(10);
    }

    // After 100 calls with 10ms between each, total elapsed = 1000ms
    // But each call resets the timer, so only last call should have fired
    // Actually after loop, we've advanced 1000ms and last call was at 990ms
    // So fn should have been called with argument 99 at time 1090ms

    vi.advanceTimersByTime(100);
    expect(fn).toHaveBeenCalledOnce();
    expect(fn).toHaveBeenCalledWith(99);

    vi.useRealTimers();
  });

  it('works with async functions', () => {
    vi.useFakeTimers();
    const asyncFn = vi.fn().mockResolvedValue('result');
    const debounced = debounce(asyncFn, 100);

    debounced();
    vi.advanceTimersByTime(100);

    expect(asyncFn).toHaveBeenCalledOnce();
    vi.useRealTimers();
  });

  it('preserves this context when called', () => {
    vi.useFakeTimers();
    const obj = {
      value: 42,
      fn: vi.fn(function (this: { value: number }) {
        return this?.value;
      }),
      debounced: null as ReturnType<typeof debounce> | null,
    };
    obj.debounced = debounce(obj.fn, 100);

    obj.debounced();
    vi.advanceTimersByTime(100);

    expect(obj.fn).toHaveBeenCalled();
    vi.useRealTimers();
  });

  it('does not call function if never triggered', () => {
    vi.useFakeTimers();
    const fn = vi.fn();
    debounce(fn, 100);

    vi.advanceTimersByTime(1000);
    expect(fn).not.toHaveBeenCalled();

    vi.useRealTimers();
  });

  it('handles long delays', () => {
    vi.useFakeTimers();
    const fn = vi.fn();
    const debounced = debounce(fn, 60000); // 1 minute

    debounced();
    vi.advanceTimersByTime(30000);
    expect(fn).not.toHaveBeenCalled();

    vi.advanceTimersByTime(30000);
    expect(fn).toHaveBeenCalledOnce();

    vi.useRealTimers();
  });
});
