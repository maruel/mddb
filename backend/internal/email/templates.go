// Provides localized email templates.

package email

import "fmt"

// Locale represents a supported language code.
type Locale string

// Supported locales for email templates.
const (
	LocaleEN Locale = "en"
	LocaleFR Locale = "fr"
	LocaleDE Locale = "de"
	LocaleES Locale = "es"
)

// DefaultLocale is used when no locale is specified or the locale is unsupported.
const DefaultLocale = LocaleEN

// ParseLocale converts a string to a Locale, returning DefaultLocale if unsupported.
func ParseLocale(s string) Locale {
	switch Locale(s) {
	case LocaleEN, LocaleFR, LocaleDE, LocaleES:
		return Locale(s)
	default:
		return DefaultLocale
	}
}

// emailTemplates holds localized email content.
type emailTemplates struct {
	// Email verification
	VerificationSubject string
	VerificationBody    string

	// Organization invitation
	OrgInvitationSubject string
	OrgInvitationBody    string

	// Workspace invitation
	WSInvitationSubject string
	WSInvitationBody    string
}

var templates = map[Locale]*emailTemplates{
	LocaleEN: {
		VerificationSubject: "Verify your email address",
		VerificationBody: `Hi %s,

Please verify your email address by clicking the link below:

%s

This link will expire in 24 hours.

If you didn't request this verification, you can safely ignore this email.

- The mddb Team
`,
		OrgInvitationSubject: "You've been invited to join %s",
		OrgInvitationBody: `Hi,

%s has invited you to join the organization "%s" as %s.

Click the link below to accept the invitation:

%s

This invitation will expire in 7 days.

If you weren't expecting this invitation, you can safely ignore this email.

- The mddb Team
`,
		WSInvitationSubject: "You've been invited to join %s",
		WSInvitationBody: `Hi,

%s has invited you to join the workspace "%s" (in organization "%s") as %s.

Click the link below to accept the invitation:

%s

This invitation will expire in 7 days.

If you weren't expecting this invitation, you can safely ignore this email.

- The mddb Team
`,
	},
	LocaleFR: {
		VerificationSubject: "Vérifiez votre adresse e-mail",
		VerificationBody: `Bonjour %s,

Veuillez vérifier votre adresse e-mail en cliquant sur le lien ci-dessous :

%s

Ce lien expirera dans 24 heures.

Si vous n'avez pas demandé cette vérification, vous pouvez ignorer cet e-mail.

- L'équipe mddb
`,
		OrgInvitationSubject: "Vous avez été invité(e) à rejoindre %s",
		OrgInvitationBody: `Bonjour,

%s vous a invité(e) à rejoindre l'organisation « %s » en tant que %s.

Cliquez sur le lien ci-dessous pour accepter l'invitation :

%s

Cette invitation expirera dans 7 jours.

Si vous n'attendiez pas cette invitation, vous pouvez ignorer cet e-mail.

- L'équipe mddb
`,
		WSInvitationSubject: "Vous avez été invité(e) à rejoindre %s",
		WSInvitationBody: `Bonjour,

%s vous a invité(e) à rejoindre l'espace de travail « %s » (dans l'organisation « %s ») en tant que %s.

Cliquez sur le lien ci-dessous pour accepter l'invitation :

%s

Cette invitation expirera dans 7 jours.

Si vous n'attendiez pas cette invitation, vous pouvez ignorer cet e-mail.

- L'équipe mddb
`,
	},
	LocaleDE: {
		VerificationSubject: "Bestätigen Sie Ihre E-Mail-Adresse",
		VerificationBody: `Hallo %s,

Bitte bestätigen Sie Ihre E-Mail-Adresse, indem Sie auf den folgenden Link klicken:

%s

Dieser Link läuft in 24 Stunden ab.

Wenn Sie diese Bestätigung nicht angefordert haben, können Sie diese E-Mail ignorieren.

- Das mddb-Team
`,
		OrgInvitationSubject: "Sie wurden eingeladen, %s beizutreten",
		OrgInvitationBody: `Hallo,

%s hat Sie eingeladen, der Organisation „%s" als %s beizutreten.

Klicken Sie auf den folgenden Link, um die Einladung anzunehmen:

%s

Diese Einladung läuft in 7 Tagen ab.

Wenn Sie diese Einladung nicht erwartet haben, können Sie diese E-Mail ignorieren.

- Das mddb-Team
`,
		WSInvitationSubject: "Sie wurden eingeladen, %s beizutreten",
		WSInvitationBody: `Hallo,

%s hat Sie eingeladen, dem Arbeitsbereich „%s" (in der Organisation „%s") als %s beizutreten.

Klicken Sie auf den folgenden Link, um die Einladung anzunehmen:

%s

Diese Einladung läuft in 7 Tagen ab.

Wenn Sie diese Einladung nicht erwartet haben, können Sie diese E-Mail ignorieren.

- Das mddb-Team
`,
	},
	LocaleES: {
		VerificationSubject: "Verifica tu dirección de correo electrónico",
		VerificationBody: `Hola %s,

Por favor, verifica tu dirección de correo electrónico haciendo clic en el siguiente enlace:

%s

Este enlace caducará en 24 horas.

Si no solicitaste esta verificación, puedes ignorar este correo electrónico.

- El equipo de mddb
`,
		OrgInvitationSubject: "Has sido invitado/a a unirte a %s",
		OrgInvitationBody: `Hola,

%s te ha invitado a unirte a la organización "%s" como %s.

Haz clic en el siguiente enlace para aceptar la invitación:

%s

Esta invitación caducará en 7 días.

Si no esperabas esta invitación, puedes ignorar este correo electrónico.

- El equipo de mddb
`,
		WSInvitationSubject: "Has sido invitado/a a unirte a %s",
		WSInvitationBody: `Hola,

%s te ha invitado a unirte al espacio de trabajo "%s" (en la organización "%s") como %s.

Haz clic en el siguiente enlace para aceptar la invitación:

%s

Esta invitación caducará en 7 días.

Si no esperabas esta invitación, puedes ignorar este correo electrónico.

- El equipo de mddb
`,
	},
}

// getTemplates returns templates for the given locale, falling back to English.
func getTemplates(locale Locale) *emailTemplates {
	if t, ok := templates[locale]; ok {
		return t
	}
	return templates[DefaultLocale]
}

// VerificationEmail returns localized subject and body for email verification.
func VerificationEmail(locale Locale, name, verifyURL string) (subject, body string) {
	t := getTemplates(locale)
	return t.VerificationSubject, fmt.Sprintf(t.VerificationBody, name, verifyURL)
}

// OrgInvitationEmail returns localized subject and body for organization invitation.
func OrgInvitationEmail(locale Locale, orgName, inviterName, role, acceptURL string) (subject, body string) {
	t := getTemplates(locale)
	return fmt.Sprintf(t.OrgInvitationSubject, orgName),
		fmt.Sprintf(t.OrgInvitationBody, inviterName, orgName, role, acceptURL)
}

// WSInvitationEmail returns localized subject and body for workspace invitation.
func WSInvitationEmail(locale Locale, wsName, orgName, inviterName, role, acceptURL string) (subject, body string) {
	t := getTemplates(locale)
	return fmt.Sprintf(t.WSInvitationSubject, wsName),
		fmt.Sprintf(t.WSInvitationBody, inviterName, wsName, orgName, role, acceptURL)
}
