// Error boundary component to prevent full app crashes.

import { ErrorBoundary as SolidErrorBoundary, type ParentComponent } from 'solid-js';
import styles from './ErrorBoundary.module.css';

interface ErrorFallbackProps {
  error: Error;
  reset: () => void;
}

function ErrorFallback(props: ErrorFallbackProps) {
  return (
    <div class={styles.errorBoundary} role="alert">
      <h2>Something went wrong</h2>
      <p class={styles.errorMessage}>{props.error.message}</p>
      <details class={styles.errorDetails}>
        <summary>Technical details</summary>
        <pre>{props.error.stack}</pre>
      </details>
      <button class={styles.retryButton} onClick={() => props.reset()}>
        Try again
      </button>
    </div>
  );
}

export const AppErrorBoundary: ParentComponent = (props) => {
  return (
    <SolidErrorBoundary fallback={(err, reset) => <ErrorFallback error={err} reset={reset} />}>
      {props.children}
    </SolidErrorBoundary>
  );
};

export default AppErrorBoundary;
