// Tests for shared dialog and menu keyboard and focus behavior.

import { afterEach, describe, expect, it } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@solidjs/testing-library';
import { createSignal, Show } from 'solid-js';
import { Dialog } from './Dialog';
import { IconButton } from './IconButton';
import { Menu, MenuItem } from './Menu';

afterEach(() => {
  cleanup();
});

function DialogHarness(props: { dismissOnBackdrop: boolean; dismissOnEscape: boolean }) {
  const [isOpen, setIsOpen] = createSignal(false);

  return (
    <>
      <button type="button" onClick={() => setIsOpen(true)}>
        Open dialog
      </button>
      <Show when={isOpen()}>
        <Dialog
          ariaLabel="Test dialog"
          dismissOnBackdrop={props.dismissOnBackdrop}
          dismissOnEscape={props.dismissOnEscape}
          onClose={() => setIsOpen(false)}
        >
          <button type="button">First dialog action</button>
          <button type="button">Last dialog action</button>
        </Dialog>
      </Show>
    </>
  );
}

function MenuHarness() {
  const [isOpen, setIsOpen] = createSignal(false);
  let triggerRef: HTMLButtonElement | undefined;

  return (
    <>
      <button ref={(el) => (triggerRef = el)} type="button" onClick={() => setIsOpen(!isOpen())}>
        Open menu
      </button>
      <Show when={isOpen()}>
        <Menu ariaLabel="Test menu" onClose={() => setIsOpen(false)} trigger={() => triggerRef}>
          <MenuItem>First action</MenuItem>
          <MenuItem>Second action</MenuItem>
          <MenuItem>Third action</MenuItem>
        </Menu>
      </Show>
    </>
  );
}

describe('Dialog', () => {
  it('restores trigger focus after Escape dismissal', async () => {
    render(() => <DialogHarness dismissOnBackdrop={true} dismissOnEscape={true} />);
    const trigger = screen.getByRole('button', { name: 'Open dialog' });

    trigger.focus();
    fireEvent.click(trigger);

    await waitFor(() => expect(screen.getByRole('dialog')).toBeTruthy());
    expect(screen.getByRole('button', { name: 'First dialog action' })).toHaveFocus();

    fireEvent.keyDown(document, { key: 'Escape' });

    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(trigger).toHaveFocus();
  });

  it('dismisses on a permitted backdrop click', async () => {
    render(() => <DialogHarness dismissOnBackdrop={true} dismissOnEscape={true} />);
    fireEvent.click(screen.getByRole('button', { name: 'Open dialog' }));

    const dialog = await screen.findByRole('dialog');
    const backdrop = dialog.parentElement;
    if (!backdrop) throw new Error('Dialog backdrop was not rendered');
    fireEvent.click(backdrop);

    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
  });

  it('keeps a non-dismissible dialog open for Escape and backdrop clicks', async () => {
    render(() => <DialogHarness dismissOnBackdrop={false} dismissOnEscape={false} />);
    fireEvent.click(screen.getByRole('button', { name: 'Open dialog' }));

    const dialog = await screen.findByRole('dialog');
    const escapeWasHandled = !fireEvent.keyDown(document, { key: 'Escape' });
    fireEvent.click(dialog.parentElement as HTMLElement);

    expect(escapeWasHandled).toBe(true);
    expect(screen.getByRole('dialog')).toBeTruthy();
  });

  it('traps Tab from the first, last, and outside focus positions', async () => {
    render(() => <DialogHarness dismissOnBackdrop={true} dismissOnEscape={true} />);
    const trigger = screen.getByRole('button', { name: 'Open dialog' });
    fireEvent.click(trigger);

    await screen.findByRole('dialog');
    const first = screen.getByRole('button', { name: 'First dialog action' });
    const last = screen.getByRole('button', { name: 'Last dialog action' });

    fireEvent.keyDown(document, { key: 'Tab', shiftKey: true });
    expect(last).toHaveFocus();
    fireEvent.keyDown(document, { key: 'Tab' });
    expect(first).toHaveFocus();

    trigger.focus();
    fireEvent.keyDown(document, { key: 'Tab' });
    expect(first).toHaveFocus();
    trigger.focus();
    fireEvent.keyDown(document, { key: 'Tab', shiftKey: true });
    expect(last).toHaveFocus();
  });
});

describe('Menu', () => {
  it('moves focus with arrow keys and restores focus on Escape', async () => {
    render(() => <MenuHarness />);
    const trigger = screen.getByRole('button', { name: 'Open menu' });

    trigger.focus();
    fireEvent.click(trigger);

    const menu = await screen.findByRole('menu', { name: 'Test menu' });
    const first = screen.getByRole('menuitem', { name: 'First action' });
    const second = screen.getByRole('menuitem', { name: 'Second action' });
    const third = screen.getByRole('menuitem', { name: 'Third action' });

    expect(first).toHaveFocus();
    fireEvent.keyDown(menu, { key: 'ArrowDown' });
    expect(second).toHaveFocus();
    fireEvent.keyDown(menu, { key: 'ArrowUp' });
    expect(first).toHaveFocus();
    fireEvent.keyDown(menu, { key: 'ArrowUp' });
    expect(third).toHaveFocus();
    fireEvent.keyDown(menu, { key: 'Home' });
    expect(first).toHaveFocus();
    fireEvent.keyDown(menu, { key: 'End' });
    expect(third).toHaveFocus();
    fireEvent.keyDown(menu, { key: 'Escape' });

    await waitFor(() => expect(screen.queryByRole('menu')).toBeNull());
    expect(trigger).toHaveFocus();
  });
});

describe('IconButton', () => {
  it('requires and exposes its accessible name', () => {
    render(() => (
      <IconButton aria-label="Close record">
        <svg aria-hidden="true" />
      </IconButton>
    ));

    expect(screen.getByRole('button', { name: 'Close record' })).toBeTruthy();
  });
});
