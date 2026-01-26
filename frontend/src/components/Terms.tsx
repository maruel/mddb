// Terms of Service page component.

import styles from './Terms.module.css';

export default function Terms() {
  return (
    <div class={styles.container}>
      <div class={styles.content}>
        <header class={styles.header}>
          <h1>Terms of Service</h1>
          <p>Last updated: January 18, 2026</p>
        </header>

        <section class={styles.section}>
          <h2>1. Agreement to Terms</h2>
          <p>
            By accessing our service, you agree to be bound by these Terms of Service. If you do not agree to these
            terms, please do not use our service.
          </p>
        </section>

        <section class={styles.section}>
          <h2>2. Intellectual Property Rights</h2>
          <p>
            Unless otherwise indicated, the Site is our proprietary property and all source code, data, functionality,
            software, website designs, audio, video, text, photographs, and graphics on the Site (collectively, the
            “Content”) and the trademarks, service marks, and logos contained therein (the “Marks”) are owned or
            controlled by us or licensed to us.
          </p>
        </section>

        <section class={styles.section}>
          <h2>3. User Representations</h2>
          <p>
            By using the Site, you represent and warrant that: (1) all registration information you submit will be true,
            accurate, current, and complete; (2) you will maintain the accuracy of such information and promptly update
            such registration information as necessary.
          </p>
        </section>

        <section class={styles.section}>
          <h2>4. Prohibited Activities</h2>
          <p>
            You may not access or use the Site for any purpose other than that for which we make the Site available. The
            Site may not be used in connection with any commercial endeavors except those that are specifically endorsed
            or approved by us.
          </p>
        </section>

        <section class={styles.section}>
          <h2>5. Limitation of Liability</h2>
          <p>
            In no event will we or our directors, employees, or agents be liable to you or any third party for any
            direct, indirect, consequential, exemplary, incidental, special, or punitive damages, including lost profit,
            lost revenue, loss of data, or other damages arising from your use of the site.
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
