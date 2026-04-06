# ATConnect — What It Is and Why It Matters

## The Short Version

ATConnect lets people sign in to websites and apps using their **Bluesky / AT Protocol identity**, the same way you might "Sign in with Google" or "Sign in with Apple" today. Except instead of relying on a big tech company to vouch for who you are, your identity lives on the open, decentralized AT Protocol network. You own it.

## Wait, What Problem Does This Solve?

You've probably clicked a "Sign in with Google" button before. When you do that, the website you're visiting asks Google, "Hey, is this person really who they say they are?" Google says yes, hands over a few details (like your name and email), and you're in. You never had to create another account with yet another password.

That system is called **OpenID Connect** (OIDC), and it's become the quiet backbone of how identity works on the modern web. Thousands of services use it — from company intranets to cloud platforms like AWS and Cloudflare.

The catch? It puts a corporation like Google, Microsoft, or Apple at the center of your identity. If they lock your account, or change their policies, or just go down for maintenance, you can lose access to everything that depends on them.  As you can imagine, there are many more privacy concerns in this model.

**ATConnect flips that around.** Instead of a corporation, your identity comes from the **AT Protocol**.  Thats the thing behind Bluesky. Your identity is tied to your handle (like `yourname.bsky.social` or even a custom domain like `alice.example.com`) and backed by a cryptographic identifier called a **DID** that nobody can take away from you. You can even host it yourself. ATConnect takes that identity and translates it into the OIDC language that thousands of existing services already understand.

In plain terms: **ATConnect is a bridge between your decentralized AT Protocol identity and the rest of the web.**

---

## How Is This Useful?

### For Distributed Teams and Communities

Imagine you're part of an open-source project or a creative collective scattered across the world. You don't all work at the same company. You don't share a corporate Google Workspace or Microsoft 365 tenant. But you still need to:

- **Control who can access your internal wiki or documentation site.** With ATConnect, you can say "only people with these Bluesky handles can view the team docs"
- **Gate access to a private blog or newsletter.** Maybe you write posts that are only for project contributors, paying supporters, or close collaborators. ATConnect can verify someone's identity so your blog platform knows whether to show them the secret posts or not.

- **Protect cloud infrastructure.** Services like Cloudflare Access and AWS IAM Identity Center already speak OIDC. ATConnect plugs right in, so you can use AT Protocol handles to decide who gets into your staging environment, admin dashboards, or deployment pipelines.

- **Collaborate across organizational boundaries.** Because AT Protocol identities aren't owned by any single organization, people from different teams, companies, or communities can authenticate to shared resources without anyone needing to be "in the same org." It's identity that travels with *you*, not with your employer.

### For Individuals

- **One identity, many places.** If you already have a Bluesky account, you already have an AT Protocol identity. ATConnect means that same identity can get you into any service that supports OIDC.

- **Custom domains as identity.** On AT Protocol, you can use your own domain as your handle (e.g., `alice.example.com`). That means your online identity is truly yours — not rented from a platform — and ATConnect makes that portable identity usable everywhere.

## How Does It Work? (The Non-Technical Version)

Here's what happens when you sign in to a website that uses ATConnect:

1. **You visit a website** and click "Sign in."
2. **You enter your AT Protocol handle** — something like `yourname.bsky.social`.
3. **ATConnect talks to your Personal Data Server (PDS)** to verify that you really control that identity. This happens through a secure process — you might see a familiar-looking authorization screen, similar to when Google asks "Do you want to allow this app to access your account?"
4. **Once verified, ATConnect issues a token** — a small, piece of data that says "this person is `yourname.bsky.social`, and their DID is `did:plc:abc123`."
5. **The website reads that token** and knows who you are. No new password, no new account, no big tech company in the middle.

That token follows the OIDC standard, so it works with all the same tools and services that already accept "Sign in with Google" — the website doesn't even need to know anything about AT Protocol. ATConnect handles the translation.


## Current Status

ATConnect is in **early development**. The core authentication flow works, and the OIDC provider endpoints are functional, but it's not yet production-ready. If you're a developer interested in decentralized identity, this is a great time to get involved, try it out, and help shape where it goes.

Check out the [README](README.md) for setup instructions and technical details.
