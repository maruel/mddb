import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor, cleanup } from '@solidjs/testing-library';
import type { JSX } from 'solid-js';
import { Router, Route } from '@solidjs/router';
import Auth from './Auth';
import { I18nProvider } from '../i18n';
import type { UserResponse, AuthResponse, ErrorResponse } from '@sdk/types.gen';

// Mock CSS module
vi.mock('./Auth.module.css', () => ({
  default: {
    authContainer: 'authContainer',
    authForm: 'authForm',
    error: 'error',
    formGroup: 'formGroup',
    toggle: 'toggle',
    oauthSection: 'oauthSection',
    divider: 'divider',
    oauthButtons: 'oauthButtons',
    googleButton: 'googleButton',
    microsoftButton: 'microsoftButton',
    oauthButton: 'oauthButton',
    authFooter: 'authFooter',
  },
}));

// Mock response for providers endpoint
const mockProvidersResponse = {
  ok: true,
  json: () => Promise.resolve({ providers: ['google', 'microsoft'] }),
};

// Mock fetch
const mockFetch = vi.fn();
globalThis.fetch = mockFetch;

// Note: @solidjs/router uses browser's History API directly.
// We spy on history methods but don't mock them since the router needs real navigation.
const historyPushStateSpy = vi.spyOn(window.history, 'pushState');

function renderWithProviders(component: () => JSX.Element) {
  return render(() => (
    <Router>
      <Route path="*" component={() => <I18nProvider>{component()}</I18nProvider>} />
    </Router>
  ));
}

describe('Auth', () => {
  const mockOnLogin = vi.fn();

  beforeEach(() => {
    cleanup();
    vi.clearAllMocks();
    mockFetch.mockReset();
    historyPushStateSpy.mockClear();
    // Default mock for providers endpoint (called on component mount)
    mockFetch.mockImplementation((url: string) => {
      if (url === '/api/v1/auth/providers') {
        return Promise.resolve(mockProvidersResponse);
      }
      return Promise.reject(new Error(`Unexpected fetch to ${url}`));
    });
  });

  afterEach(() => {
    cleanup();
    vi.restoreAllMocks();
  });

  it('renders login form by default', async () => {
    renderWithProviders(() => <Auth onLogin={mockOnLogin} />);

    await waitFor(() => {
      expect(screen.getByLabelText(/email/i)).toBeTruthy();
      expect(screen.getByLabelText(/password/i)).toBeTruthy();
    });

    // Name field should not be visible in login mode
    expect(screen.queryByLabelText(/^name$/i)).toBeFalsy();
  });

  it('switches to register form when clicking register link', async () => {
    renderWithProviders(() => <Auth onLogin={mockOnLogin} />);

    await waitFor(() => {
      expect(screen.getByText(/register/i)).toBeTruthy();
    });

    // Find and click the register button/link
    const registerButtons = screen.getAllByText(/register/i);
    const toggleButton = registerButtons.find(
      (el) => el.tagName.toLowerCase() === 'button' && el.getAttribute('type') === 'button'
    );
    if (toggleButton) {
      fireEvent.click(toggleButton);
    }

    await waitFor(() => {
      // Name field should now be visible
      expect(screen.getByLabelText(/^name$/i)).toBeTruthy();
    });
  });

  it('handles successful login', async () => {
    const mockUser: UserResponse = {
      id: 'user-1',
      email: 'test@example.com',
      name: 'Test User',
      settings: { theme: 'light', language: 'en' },
      created: 1704067200,
      modified: 1704067200,
    };

    const mockResponse: AuthResponse = {
      token: 'test-token-123',
      user: mockUser,
    };

    mockFetch.mockImplementation((url: string) => {
      if (url === '/api/v1/auth/providers') {
        return Promise.resolve(mockProvidersResponse);
      }
      if (url === '/api/v1/auth/login') {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve(mockResponse),
        });
      }
      return Promise.reject(new Error(`Unexpected fetch to ${url}`));
    });

    renderWithProviders(() => <Auth onLogin={mockOnLogin} />);

    await waitFor(() => {
      expect(screen.getByLabelText(/email/i)).toBeTruthy();
    });

    // Fill in the form
    fireEvent.input(screen.getByLabelText(/email/i), {
      target: { value: 'test@example.com' },
    });
    fireEvent.input(screen.getByLabelText(/password/i), {
      target: { value: 'password123' },
    });

    // Submit the form
    const submitButton = screen.getByRole('button', { name: /login/i });
    fireEvent.click(submitButton);

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith('/api/v1/auth/login', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email: 'test@example.com', password: 'password123' }),
      });
    });

    await waitFor(() => {
      expect(mockOnLogin).toHaveBeenCalledWith('test-token-123', mockUser);
    });
  });

  it('handles login error', async () => {
    const errorResponse: ErrorResponse = {
      error: {
        code: 'UNAUTHORIZED',
        message: 'Invalid credentials',
      },
    };

    mockFetch.mockImplementation((url: string) => {
      if (url === '/api/v1/auth/providers') {
        return Promise.resolve(mockProvidersResponse);
      }
      if (url === '/api/v1/auth/login') {
        return Promise.resolve({
          ok: false,
          json: () => Promise.resolve(errorResponse),
        });
      }
      return Promise.reject(new Error(`Unexpected fetch to ${url}`));
    });

    renderWithProviders(() => <Auth onLogin={mockOnLogin} />);

    await waitFor(() => {
      expect(screen.getByLabelText(/email/i)).toBeTruthy();
    });

    fireEvent.input(screen.getByLabelText(/email/i), {
      target: { value: 'test@example.com' },
    });
    fireEvent.input(screen.getByLabelText(/password/i), {
      target: { value: 'wrongpassword' },
    });

    const submitButton = screen.getByRole('button', { name: /login/i });
    fireEvent.click(submitButton);

    await waitFor(() => {
      expect(screen.getByText('Invalid credentials')).toBeTruthy();
    });

    expect(mockOnLogin).not.toHaveBeenCalled();
  });

  it('handles network error', async () => {
    mockFetch.mockImplementation((url: string) => {
      if (url === '/api/v1/auth/providers') {
        return Promise.resolve(mockProvidersResponse);
      }
      return Promise.reject(new Error('Network error'));
    });

    renderWithProviders(() => <Auth onLogin={mockOnLogin} />);

    await waitFor(() => {
      expect(screen.getByLabelText(/email/i)).toBeTruthy();
    });

    fireEvent.input(screen.getByLabelText(/email/i), {
      target: { value: 'test@example.com' },
    });
    fireEvent.input(screen.getByLabelText(/password/i), {
      target: { value: 'password123' },
    });

    const submitButton = screen.getByRole('button', { name: /login/i });
    fireEvent.click(submitButton);

    await waitFor(() => {
      expect(screen.getByText(/error occurred/i)).toBeTruthy();
    });

    expect(mockOnLogin).not.toHaveBeenCalled();
  });

  it('handles successful registration', async () => {
    const mockUser: UserResponse = {
      id: 'user-1',
      email: 'new@example.com',
      name: 'New User',
      settings: { theme: 'light', language: 'en' },
      created: 1704067200,
      modified: 1704067200,
    };

    const mockResponse: AuthResponse = {
      token: 'new-token-123',
      user: mockUser,
    };

    mockFetch.mockImplementation((url: string) => {
      if (url === '/api/v1/auth/providers') {
        return Promise.resolve(mockProvidersResponse);
      }
      if (url === '/api/v1/auth/register') {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve(mockResponse),
        });
      }
      return Promise.reject(new Error(`Unexpected fetch to ${url}`));
    });

    renderWithProviders(() => <Auth onLogin={mockOnLogin} />);

    await waitFor(() => {
      expect(screen.getByText(/register/i)).toBeTruthy();
    });

    // Switch to register mode
    const registerButtons = screen.getAllByText(/register/i);
    const toggleButton = registerButtons.find(
      (el) => el.tagName.toLowerCase() === 'button' && el.getAttribute('type') === 'button'
    );
    if (toggleButton) {
      fireEvent.click(toggleButton);
    }

    await waitFor(() => {
      expect(screen.getByLabelText(/^name$/i)).toBeTruthy();
    });

    // Fill in the registration form
    fireEvent.input(screen.getByLabelText(/^name$/i), {
      target: { value: 'New User' },
    });
    fireEvent.input(screen.getByLabelText(/email/i), {
      target: { value: 'new@example.com' },
    });
    fireEvent.input(screen.getByLabelText(/password/i), {
      target: { value: 'newpassword123' },
    });

    // Submit the form - find the submit button
    const submitButtons = screen.getAllByRole('button');
    const submitButton = submitButtons.find((btn) => btn.getAttribute('type') === 'submit');
    if (submitButton) {
      fireEvent.click(submitButton);
    }

    await waitFor(() => {
      expect(mockFetch).toHaveBeenCalledWith('/api/v1/auth/register', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          email: 'new@example.com',
          password: 'newpassword123',
          name: 'New User',
        }),
      });
    });
  });

  it('shows OAuth login buttons', async () => {
    renderWithProviders(() => <Auth onLogin={mockOnLogin} />);

    await waitFor(() => {
      expect(screen.getByText(/google/i)).toBeTruthy();
      expect(screen.getByText(/microsoft/i)).toBeTruthy();
    });

    // Check OAuth links
    const googleLink = screen.getByText(/google/i);
    expect(googleLink.getAttribute('href')).toBe('/api/v1/auth/oauth/google');

    const microsoftLink = screen.getByText(/microsoft/i);
    expect(microsoftLink.getAttribute('href')).toBe('/api/v1/auth/oauth/microsoft');
  });

  it('shows privacy policy link', async () => {
    renderWithProviders(() => <Auth onLogin={mockOnLogin} />);

    await waitFor(() => {
      expect(screen.getByText(/privacy/i)).toBeTruthy();
    });

    // Verify the privacy link is rendered with correct href
    const privacyLink = screen.getByText(/privacy/i);
    expect(privacyLink.getAttribute('href')).toBe('/privacy');
    // Note: Clicking triggers browser navigation which is covered by e2e tests
  });

  it('disables submit button while loading', async () => {
    // Make login fetch hang
    mockFetch.mockImplementation((url: string) => {
      if (url === '/api/v1/auth/providers') {
        return Promise.resolve(mockProvidersResponse);
      }
      // Make login request hang
      return new Promise((resolve) => setTimeout(resolve, 10000));
    });

    renderWithProviders(() => <Auth onLogin={mockOnLogin} />);

    await waitFor(() => {
      expect(screen.getByLabelText(/email/i)).toBeTruthy();
    });

    fireEvent.input(screen.getByLabelText(/email/i), {
      target: { value: 'test@example.com' },
    });
    fireEvent.input(screen.getByLabelText(/password/i), {
      target: { value: 'password123' },
    });

    const submitButton = screen.getByRole('button', { name: /login/i });
    fireEvent.click(submitButton);

    await waitFor(() => {
      expect(submitButton).toHaveProperty('disabled', true);
    });
  });

  it('handles response without token or user', async () => {
    mockFetch.mockImplementation((url: string) => {
      if (url === '/api/v1/auth/providers') {
        return Promise.resolve(mockProvidersResponse);
      }
      if (url === '/api/v1/auth/login') {
        return Promise.resolve({
          ok: true,
          json: () => Promise.resolve({}), // Empty response
        });
      }
      return Promise.reject(new Error(`Unexpected fetch to ${url}`));
    });

    renderWithProviders(() => <Auth onLogin={mockOnLogin} />);

    await waitFor(() => {
      expect(screen.getByLabelText(/email/i)).toBeTruthy();
    });

    fireEvent.input(screen.getByLabelText(/email/i), {
      target: { value: 'test@example.com' },
    });
    fireEvent.input(screen.getByLabelText(/password/i), {
      target: { value: 'password123' },
    });

    const submitButton = screen.getByRole('button', { name: /login/i });
    fireEvent.click(submitButton);

    await waitFor(() => {
      expect(screen.getByText(/invalid response/i)).toBeTruthy();
    });

    expect(mockOnLogin).not.toHaveBeenCalled();
  });
});
