---
source: https://www.flightcontrol.dev/blog/ultimate-guide-to-multi-tenant-saas-data-modeling
---

# Ultimate guide to multi-tenant SaaS data modeling

**Author:** Brandon Bayer

**TLDR:** if you're considering multi-tenant data, which all SaaS builders must, you should build "teams" functionality from day one. This guide explains how (and why).

You should build in "teams" functionality on day one if you don't want to regret the past. You will need teams functionality eventually. Unless, maybe, you are building a super basic mobile or desktop app. But what about something that seems simple, like a bookmark manager? You probably still need it! Your users will probably want a family account, a business team account, or simply to share access with an assistant.

As a user of the internet, I have too often been frustrated with a piece of software because they didn't add teams functionality on day one. It's very complex and painful to add later, so I have to wait many months or years for them to finally add it.

Humans are relational beings. When creating software, you model the world. If you don't model the concept of human relationships, you create an impedance mismatch.

It's quick and easy to add on day one. You can add it later if you have to, using database expand-and-contract patterns, but it's always easier to do it now.

This post aims to be the most comprehensive guide to multi-tenant data modeling. This is based on my 14+ years in tech, building multiple SaaS apps, and comparing notes with other SaaS builders.

## Multi-tenant SaaS fundamentals

**SaaS** is software-as-a-service and is probably what you work on.

**Multi-tenant** means you have multiple users of your app that you want to keep separate. For example, a multi-tenant apartment building contains multiple tenants. All tenants have access to shared common spaces or resources. Each tenant's private residence has its own access control. Each tenant likely has multiple people who are permitted in their private space, so they need extra keys they can give to others who need access.

**Single database vs database per user:** there are two main ways to achieve data isolation for each tenant. You can have a single, shared database where every table has an `organization_id` and every read and write will include `organization_id`. Or you can create a separate database for each user with fully separate database instances or different schemas inside a single database.

**You should default to a single, shared database unless you have an exceptional case that demands otherwise.** Most people use this model. Database per user could be required in some cases for data isolation. I have no experience with the database per user approach, so I can't speak in depth about that.

## Multi-tenant data modeling

At the most basic level, we need two models, `User` and `[top level tenant]`.

### What to name the top level thing?

You can choose whatever makes you happy. The most common and obvious options are:

- **Account** — not ideal, because many people associate "account" with "user account". And you may want to use "account" for another domain model, like a bank account.
- **Team** — not ideal, because you may want to have multiple teams inside the tenant, like multiple product teams in a business.
- **Company** — not ideal, because the definition is "a commercial business," and all your tenants may not be commercial businesses.
- **Tenant** — clear and short, but it isn't common parlance with B2B customers. If you use this internally, you probably want a different user facing word.
- **Workspace** — becoming more common. It's a more natural term for single user tenants than Team or Organization. However, you might want to have multiple workspaces within an organization.
- **Organization** — what I use and recommend because it's a general term for a body of people that is most closely aligned with the concept of a tenant. And then later, you can add Teams and Workspaces within the Organization.

In this guide, I will use `Organization` for the top level model.

## GitHub, Google, or Linear access models?

There are three main access models.

- **GitHub**: a person has a single user account that is added or removed from any number of other organizations.
- **Google**: each organization requires a separate user account. There is no concept of sharing a user account across organizations.
- **Linear**: a hybrid of the other two. A single user account can have access to multiple organizations, and a person can have multiple Linear user accounts, each with different organizations it can access.

The clever ones in the room will point out that you can create a new GitHub account for each organization, but that's not the normal or common path.

### The GitHub model

In the GitHub model, a user can have access to multiple organizations from a single user account. Inviting a user to an organization will always invite an existing user. **It will never create a new user account.**

The GitHub model is ideal for social oriented apps where a user has a strong identity, accumulated personal information attached to their account, and public user profiles.

This model has significant challenges when it comes to enterprise customers who want full control over user accounts, for example with SAML SSO. Because they can't control creation or deletion of that personal user account. However, GitHub Enterprise now has [an option to provision new accounts](https://docs.github.com/en/enterprise-cloud@latest/admin/identity-and-access-management/understanding-iam-for-enterprises/choosing-an-enterprise-type-for-github-enterprise-cloud) for access to the organization. If you take this into account, GitHub is the same as the Linear model.

Most people will be better off with the Linear model.

### The Google model

In the Google model, a user cannot have access to multiple organizations. Inviting a user to an organization will always **create** a new user.

The Google model is ideal for email providers and enterprise business-to-business (B2B) apps with strict access models where anyone, like consultants, interacting with that business will be given their own business email. In this case, there is no value in sharing user identity across organizations, and it doesn't make sense for a single email to have access to multiple organizations.

However, this breaks down at scale, even for enterprises, because eventually some users will need access to multiple organizations, like managed service providers, during or after mergers, and acquisitions.

It's also frustrating for small accounts, and you end up with hacks like `name+development@work.com`.

Concurrent user sessions is important for a good user experience and is discussed below.

Most people will be better off with the Linear model.

### The Linear model

In the Linear model, a user can both have separate user accounts for different organizations and have access to multiple organizations from a single account.

The Linear model combines the best of GitHub and Google and is ideal for most B2B startups because it supports both enterprise and small companies using consultants. Enterprises can both strictly control user provisioning and access and allow their users to belong to multiple organizations. It is the most flexible, and it provides the best user experience because it reduces the number of logins required to access all their organizations.

I recommend the Linear model for most startups. You need to really know what you are doing and why you are doing it before choosing the GitHub or Google models.

## Personal vs business organization accounts

For apps that will likely be used for both personal and business use, you may think of having separate personal vs business organization types. For social oriented apps like the GitHub model this could make sense. And for other models with many single-player users, a personal account enables you to offer a simpler UX by hiding things they don't care about. But there's also an argument for not hiding those and keeping them in the UI as upgrade prompts.

For Google and Linear models, you only need a single organization type. **Personal accounts are just organizations with a single user.** This makes it easier for you to implement, and it eliminates any friction should a personal account want to upgrade to a paid business account.

## Two-sided systems like marketplaces

If you have a two-sided system, like a marketplace, you will want to support the user to participate in both sides of the system without having to log out and log back in.

Another example is an enterprise app with different employer vs employee experiences. Employers are usually also employees of the legal entity.

Modeling this can be very domain specific and outside the scope of this article.

## Resource ownership

The resources in your domain will usually belong to one of three things:

- the entire system
- an organization
- a user

Things that belong to the entire system span across multiple tenants, like global data or config.

Almost everything else should belong to an organization.

Assigning an item to a user is a special case discussed further below.

## Associate users with organizations

### Requirements

- organizations can have multiple users
- user can belong to multiple organizations
- user should have an access role that can be different for every organization
- you can invite users to an organization
- you can assign items, like todos, to a user
- you can assign items to a user before they accept an invitation

### Memberships

I recommend the following membership model because it will work great for all three access models, even if a user only belongs to a single organization, like with Google.

```sql
CREATE TABLE Organization (
    id VARCHAR(255) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE Membership (
    id VARCHAR(255) PRIMARY KEY,
    role VARCHAR(255),
    
    user_id VARCHAR(255),
    organization_id VARCHAR(255) NOT NULL,
    invited_by_id VARCHAR(255),

    FOREIGN KEY (user_id) REFERENCES User(id),
    FOREIGN KEY (invited_by_id) REFERENCES User(id),
    FOREIGN KEY (organization_id) REFERENCES Organization(id)
);

CREATE TABLE User (
    id VARCHAR(255) PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

## Associate domain models with organizations

The most important thing in a multi-tenant system is data isolation between tenants.

The best way to achieve this is to:

- **Add** `**organization_id**` **on every table in your database except for users.**
- Scope all database reads and writes to a specific organization.

```javascript
const project = await prisma.project.findFirst({
  where: {
    id: projectId,
    organization_id: organizationId,
  }
})
```

Yes, even items that might be "assigned" to a user, like a todo, should also have `organization_id`.

Is adding `organization_id` on every model dangerously redundant? For example, if you have a Product model with a child ProductVariant model, theoretically you could create a ProductVariant with an `organization_id` that is different from the id on the Product. Unfortunately, there are no perfect solutions to prevent developers from introducing bugs. This is where tests and code reviews come into play. In practice, this particular issue is unlikely to occur if you are rigorous about passing `organization_id` everywhere as I recommend below.

### Loose enforcement

The loose approach is to keep `id` as the unique primary key and add `organization_id` as a separate indexed foreign key.

```sql
CREATE TABLE Project (
    id VARCHAR(255) PRIMARY KEY,
    organization_id VARCHAR(255) NOT NULL,
    
    name VARCHAR(255) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
        
    FOREIGN KEY (organization_id) REFERENCES Organization(id)
);
-- Add an index to the organization_id column
CREATE INDEX idx_organization_id ON Project(organization_id);
```

The benefit of this approach is that you can efficiently look up a domain model with just the `id`, even when you don't know what organization it belongs to.

The downside is that you could accidentally forget to scope a query with `organization_id`. To compensate, you should implement an access layer in your code that requires you to provide `organization_id` for each query.

### Strict enforcement

The strict approach is to use a compound primary key of `(organization_id, id)`.

```sql
CREATE TABLE Project (
    id VARCHAR(255) NOT NULL,
    organization_id VARCHAR(255) NOT NULL,
    
    name VARCHAR(255) NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    
    PRIMARY KEY (organization_id, id),
    FOREIGN KEY (organization_id) REFERENCES Organization(id)
);
```

The benefit of this approach is that you must include `organization_id` to identify a resource. So you never forget it. `organization_id` is first in the tuple so the index will be optimized for selecting all the entities in an organization.

Technically, with this, you could have two resources with the same id in different organizations. This unlocks the ability to provide your users with the ability to clone an organization into a sandbox environment for testing APIs or something.

## Associate domain models with users

As discussed above, all models will belong to an organization. Additionally, items in an organization can be assigned to a user. But here's the kicker: **Assign the item to the user's membership, not the user.** This allows you to assign items to a user before a user has accepted the invitation. You'll be familiar with the frustration of a lack of this if you've tried to assign GitHub issues to a user you just invited.

Beware, however, of the tendency to assign things to users that simply belong to the organization. Assigning something to a user should be very intentional, like assigning a todo to a user for them to complete. That todo would also belong to the organization with `organization_id`.

## Include the org id everywhere, including in URLs

The `organization_id` value is critical for all operations, so you should literally pass it around everywhere like it's going out of style.

- Include `organization_id` in all URLs like `/org/[orgId]/projects/[projectId]` (alternatively, use custom subdomains or domains)
- Require `organization_id` in all query inputs
- Require `organization_id` in all mutation inputs
- Require `organization_id` in most database calls (we built a small abstraction around Prisma to enforce this, but using compound keys would also solve this)

But that makes the urls "ugly!" Let me explain: when you have access to multiple organizations, and you visit a url like `/projects/123`, you will need the `organization_id` and user's `role` to properly show and hide the UI for that role. With the id in the url, you can look up the role in the session. Without this in the url, you will have to make a database call to look up the organization for this project before you can show the correct UI. Granted, you will have to fetch the project and can return the org id with that. But almost certainly you'll have some UI on the page that does not depend on the project data, but does depend on the user role. So yes, you can get by without an organization id in the url, but you have to do a lot more plumbing.

I used to be a url purist, but I'm now much happier with org id in all the urls.

## Now the tricky part: user login sessions

This is the part of the test where most companies wash out. There are many complexities with multi-tenancy, and your first attempt or two will likely be incorrect or at least have a frustrating experience for users.

This is probably the simplest with the GitHub model since you have a single user. But as you grow, you'll need enterprise level features, and GitHub Enterprise authentication couldn't be further from "simple."

### Super basic implementation

Store `organization_id` in the user session.

This effectively scopes all interactions to a single org. You can have an org switcher for switching to other orgs that you have a membership to. Switching will update the `organization_id` in the session.

But you have a big problem: Let's say I have access to org1 and org2. I'm logged into org1, try to access `/org/org2/project/1`, and then get a 404 because I'm not logged into that org. This is a terrible experience.

And if you have multiple user accounts with access to separate organizations, you have to log out and log back in to switch organizations.

Yes, this was our Flightcontrol v1 implementation.

### Less basic implementation

Store `accessibleOrgs: Array<{orgId: string, role: string}>` in the session. This must be synced across all user sessions. And if a user's access to a certain org is revoked, remove that org id from `accessibleOrgs` for all that user's sessions.

Now I can directly access all the urls of all the orgs I have access to, without having to "switch organizations".

But now you have another problem: you've started with only email/password login, and now you've added sign in with Google. One of the orgs you have access to is enforcing sign in with Google. But you have a conflict, because this one user now technically has two ways to log in, email/password and Google, and for security, they are treated as separate logins. So now you have the same problem as the 'super basic' implementation where you can only be logged in via email/password or Google, but not both at the same time.

And, still, if you have multiple user accounts with access to separate organizations, you have to log out and back in to switch organizations.

Yes, this is our Flightcontrol v2 implementation.

### Ultimate implementation: concurrent user sessions

The ultimate user experience is to never have to log out to access another organization. You can be simultaneously logged into multiple organizations with multiple identities and seamlessly switch between them.

This is how Google and Slack work. I think the world might burst into flames if they required logging out to switch accounts.

To implement, store two things in the user session:

- `accessibleOrgs: Array<{orgId: string, role: string}>`
  - should be synced across all the user's sessions
- `loggedInOrgsOnThisDevice: Array<{orgId: string}>`
  - cannot be synced across the user's sessions

The first time a user logs in on a device:

- set `accessibleOrganizations` to the list of all their org memberships
- set `loggedInOrgsOnThisDevice` to the same as `accessibleOrganizations` but filtered by orgs that permit access with the login method (like email/password vs Google)

The user can log into additional organizations by clicking "sign into organization" from the org switcher:

- add the new org id to `accessibleOrganizations`, if it's not already there, and to `loggedInOrgsOnThisDevice`

Why have separate `accessibleOrgs` and `loggedInOrgsOnThisDevice` fields? Because `accessibleOrgs` should be synced across sessions, you can change the role of or revoke access to a certain org. But `loggedInOrgsOnThisDevice` cannot be synced across devices. We want to enforce that each device must log in to the organizations it wants to access.

Yes, this will be our Flightcontrol v3 implementation.

## New user sign-up

For brand new user sign-up, in addition to creating a `User` model, you will also create an `Organization` and a `Membership` and link them all together.

Optionally you can build a feature that all users with a configured domain will automatically be added to an organization. In this case, you would create `Membership` and `User` during signup, but attach them to the existing `Organization`.

## Inviting users to an org

To invite a user to an existing organization, we'll use the `Membership` model as the invitation, along with its `invitationId`, `invitationExpiresAt`, and `invitedEmail` fields. The `Membership` table SQL is listed previously in this guide.

- create a new `Membership` and link it to the `Organization`
  - `membership.userId` will be `null`
  - `invitationId` can be CUID or other random string
- send the user an email with a link to the signup page that has `?invitationId=123` in the query parameter
- when the user submits the sign up form, include `invitationId` in the backend API call.
- in the sign-up API, if `invitationId` is included and matches a valid `Membership`, then create the `User` model and attach it to that existing `Membership`

## Revoking access to an org

To revoke a user's access to an organization, set `membership.user_id` to null or delete that membership entirely.

Keep the membership and set `user_id` to null if you want to continue to show a list in your UI of users who used to have access.

Otherwise you can delete the entire membership model, and you'll need to update any other models that were assigned to this membership to be assigned to another membership.

## Role-based access control (RBAC)

The `Membership` has a `role` or `roles` field depending on how sophisticated you want to be.

Store the `role` in the user session, so you don't have to make database or API calls to get it.

Define a list of permissions that each `role` has. This can be in code for static roles or in the database for dynamic roles. And define one or more permissions that are required to execute each query and mutation.

Use a library or build your own abstraction that allows you to check, on both client and server, whether the current `role` has a certain permission.

RBAC will probably be a full blog post itself at some point, but that's the basics.

## Billing and subscriptions belong to the organization

The `Organization` is the place to manage your billing. Typically, you'll have a `billingEmail` and `stripeCustomerId` on the `Organization`. And it can have many `Subscription`'s that have `stripeSubscriptionId` and other information.

```
model Organization {
  id 
  billingEmail
  stripeCustomerId
  
  hasMany Subscription
}
```

When you charge per user, you simply count the memberships that belong to an organization.

## Settings

There are three main setting types:

- organization can have settings
- user can have settings per organization (like notifications)
- user can have global settings (like dark mode)

Organization settings can be stored on the `Organization`.

User settings per organization can be stored on the `Membership`.

The user's global settings can be stored on `User`.

## Analytics

Oh fun, now the product person wants analytics, but seemingly every analytics provider is built around the concept of `userId` with no clear way to link multiple `userId`s to a single organization. And believe me, accurate organization level analytics are critical for multi-user systems.

Good news: it's easy to implement. It's just not usually well documented.

Most providers have the concept of a `group` that will map to `Organization`.

Here's how to make it work for Segment. (Segment is great, by the way, because it allows you to pipe analytic events to multiple analytic products). We use Segment to pipe these events to [Userlist](https://userlist.com/?via=brandon) and to [Posthog](https://posthog.com/).

```javascript
import Analytics from "analytics-node"

const orgId = '123'

// link a user to an org, and set org level traits
analytics.group({
  userId,
  groupId: orgId,
  traits: {type: "org", orgName: 'Acme', orgStatus: 'active'},
})

// submit the actual event
analytics.track({
  userId,
  event: 'project-created',
  properties: {userId, projectId: '123'},
  // must include groupId in context or it won't work
  context: {groupId: orgId},
})
```

Note that `membershipId` is not used at all here.

## Advanced extra credit

### Organization types

You could have different types of organizations, like households vs businesses and schools vs families. This could be useful for two-sided marketplaces, where each user always belongs to at least two organizations, one of each type.

### Parent and child organizations

Organizations could contain other organizations. Access could continue to be strictly isolated, but all billing could go through the parent organization. We see this approach with AWS accounts, for example.

### Organization memberships

An entire organization could be invited to collaborate as part of another organization. For example, pull in an agency to collaborate on a project.

## Further reading and inspiration

- https://blog.bullettrain.co/teams-should-be-an-mvp-feature/
- https://blitzjs.com/docs/multitenancy
- https://www.checklyhq.com/blog/building-a-multi-tenant-saas-data-model/

## Thank you's

Thank you to Andrew Culver for that OG [Bullet Train blog](https://blog.bullettrain.co/teams-should-be-an-mvp-feature/) that heavily influenced me over the years. And thank you to Agree Ahmed (NUMI), Igor Gassmann (fmr Inngest), and Mahmoud Abdelwahab (Neon) for notes and feedback!
