// Privacy policy page component.

import styles from './Privacy.module.css';

export default function Privacy() {
  return (
    <div class={styles.container}>
      <div class={styles.content}>
        <header class={styles.header}>
          <h1>Privacy Policy</h1>
          <p>Last updated: January 18, 2026</p>
        </header>

        <section class={styles.section}>
          <h2>1. Introduction</h2>
          <p>
            Welcome to mddb. We are committed to protecting your personal information and your right to privacy. If you
            have any questions or concerns about our policy, or our practices with regards to your personal information,
            please contact us.
          </p>
        </section>

        <section class={styles.section}>
          <h2>2. Information We Collect</h2>
          <p>
            We collect personal information that you voluntarily provide to us when registering at the mddb Express
            interest in obtaining information about us or our products and services, when participating in activities on
            the mddb or otherwise contacting us.
          </p>
          <p>
            The personal information that we collect depends on the context of your interactions with us and the mddb,
            the choices you make and the products and features you use.
          </p>
        </section>

        <section class={styles.section}>
          <h2>3. How We Use Your Information</h2>
          <p>
            We use personal information collected via our mddb for a variety of business purposes described below. We
            process your personal information for these purposes in reliance on our legitimate business interests, in
            order to enter into or perform a contract with you, with your consent, and/or for compliance with our legal
            obligations.
          </p>
        </section>

        <section class={styles.section}>
          <h2>4. Your Data, Your Control</h2>
          <p>
            mddb is designed to be local-first and markdown-based. Your documents and tables are stored in the location
            you specify. If you use a hosted version, the provider has access to your data.
          </p>
        </section>

        <footer class={styles.footer}>
          <button onClick={() => window.history.back()} class={styles.backButton}>
            Go Back
          </button>
        </footer>
      </div>
    </div>
  );
}
