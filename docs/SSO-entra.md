# SSO with Microsoft Entra ID (OIDC)

This guide sets up single sign-on from **Microsoft Entra ID** (formerly Azure AD)
to Orbit over OpenID Connect. Use it when your people already have Microsoft
work accounts and you want them to sign in with those, and optionally get their
Orbit role from Entra. For Authentik, see [SSO-authentik.md](SSO-authentik.md).
How SSO fits with the rest of sign-in is in [AUTH-BLUEPRINT.md](AUTH-BLUEPRINT.md).

Replace these throughout:

| Placeholder | Meaning |
|---|---|
| `orbit.example.org` | Orbit's public host name, as the browser sees it |
| `<tenant-id>` | your Entra **Directory (tenant) ID** (a GUID) |

> **The Orbit server calls Entra itself.** Discovery, the code-for-token
> exchange and the key (JWKS) check all run server-side, so the Orbit host
> needs outbound HTTPS to `login.microsoftonline.com`. The browser alone is not
> enough.

## Before you start

- Only the **owner** can change SSO settings. The **SSO** tab is under
  **Owner**, not **Admin**.
- If the owner turned on **Step-up re-authentication** (**Owner → Owner →
  Feature flags**), saving SSO settings asks you to confirm with your password
  or a passkey first. The **Test** button does not.
- Orbit must run on its public host name with HTTPS. See
  [reverse-proxies.md](reverse-proxies.md).

## 1. Register the app in Entra

In the Microsoft Entra admin center: **Identity → Applications → App
registrations → New registration**.

- **Name**: `Orbit`
- **Supported account types**: **Accounts in this organizational directory
  only** (single tenant).
- **Redirect URI**: platform **Web**, value exactly:

  ```
  https://orbit.example.org/api/v1/auth/oidc/callback
  ```

Click **Register**. On the **Overview** page, copy:

- **Application (client) ID** → Orbit's **Client ID**
- **Directory (tenant) ID** → goes into the issuer URL (step 4)

## 2. Create a client secret

**Certificates & secrets → Client secrets → New client secret**. Pick an
expiry, then copy the secret's **Value** (not the **Secret ID**). Entra shows
it only once.

> **Secrets expire.** Put the expiry date in your calendar. When it runs out,
> SSO sign-in fails with "SSO sign-in failed." until you create a new secret
> and paste it into Orbit.

## 3. Add the claims Orbit reads

Orbit always asks for the scopes `openid profile email`. It reads these
claims from the **ID token** (not from the userinfo endpoint):

| Claim | Used for |
|---|---|
| `sub` | the account key; required |
| `preferred_username` | the Orbit username (falls back to `email`, then `sub`) |
| `email` | the account's email |
| `name` | the display name |
| the role claim you choose | the roles (optional, step 5) |

Entra puts `sub`, `name` and `preferred_username` in the ID token by default.
For the email: **Token configuration → Add optional claim → ID → `email`**.
Tick the box to add the Microsoft Graph permission if Entra offers it.

> Optional: under **API permissions**, click **Grant admin consent** so users
> don't each see a consent prompt on first sign-in.

## 4. Find the issuer URL

Entra's issuer is **tenant-specific**:

```
https://login.microsoftonline.com/<tenant-id>/v2.0
```

Check it in a browser:
`https://login.microsoftonline.com/<tenant-id>/v2.0/.well-known/openid-configuration`.
The `issuer` in that JSON must match what you enter in Orbit exactly.

> Don't use `/common/v2.0` or `/organizations/v2.0`. Their discovery document
> has a templated issuer (`{tenantid}`), so the ID token's issuer never matches
> and sign-in fails.

## 5. (Optional) Give roles from Entra

Skip this if everyone should start with one fixed role. You set that as the
**Default role** in step 6.

Orbit matches the values of one claim to its **role names**. The match is
exact and case-sensitive. Entra has two ways to send such a claim.

### Option A: app roles (recommended)

App roles belong to this app only, and their values are strings you choose. So
you can name them after Orbit roles.

1. **App roles → Create app role**:
   - **Display name**: `Orbit member`
   - **Allowed member types**: **Users/Groups**
   - **Value**: `member` (must be an Orbit role name: a built-in
     `admin`, `member`, `none`, or a role you created in **Admin → Roles**)
   - Enable it. Repeat for each role you want to hand out.
2. **Enterprise applications → Orbit → Users and groups → Add user/group**:
   assign users or groups to each app role.
3. In Orbit, set **Role claim** to `roles`. Leave **Extra scopes** empty.

Entra then sends `"roles": ["member"]` in the ID token for assigned users.

### Option B: the groups claim

**Token configuration → Add groups claim**, pick **Security groups**, and make
sure it's included in the **ID** token. Set **Role claim** to `groups`.

> By default Entra sends group **object IDs** (GUIDs), not names. Orbit
> would then need roles named after those GUIDs. Users in many groups (over
> 200 for JWTs) get no `groups` claim at all (the "groups overage" case) and
> fall back to the **Default role**. Prefer app roles.

### How Orbit reads the claim

- A list of strings (Entra's `roles` and `groups`) is used as is.
- A single string is split on spaces.
- Values that match no Orbit role are ignored.
- No match at all → the **Default role**; if that's empty, the built-in
  **`none`** role (no access).

## 6. Enter the settings in Orbit

Sign in as the owner and open **Owner → SSO** (**Single sign-on (OIDC)**).

| Field | Value |
|---|---|
| (toggle, top right) | on, to enable SSO |
| **Issuer URL** | `https://login.microsoftonline.com/<tenant-id>/v2.0` |
| **Client ID** | the **Application (client) ID** from step 1 |
| **Client secret** | the secret **Value** from step 2 |
| **Redirect URL** | `https://orbit.example.org/api/v1/auth/oidc/callback` |
| **Extra scopes** | empty |
| **Role claim** | `roles` (option A), `groups` (option B), or empty |
| **Default role** | the role for new or unmatched users; empty means `none`. Use `member` if everyone should be able to create workspaces |

1. Click **Test** next to **Issuer URL**. "Discovery succeeded — issuer looks
   good" means Orbit reached Entra and found a valid OIDC provider.
2. Click **Save SSO settings**.

The **Client secret** is write-only. After saving, the field shows
`•••••••• (unchanged)`. Type into it only to replace the secret.

## 7. Sign in

Sign out. The login page now shows **Sign in with SSO**. Click it, sign in at
Microsoft, and you land back in Orbit, signed in.

## How accounts and roles work

- **Just-in-time accounts.** The first SSO sign-in creates the Orbit
  account. It is keyed on the token's `sub`, never on the email.
- **Username.** From `preferred_username` (for Entra, usually the UPN), else
  `email`, else `sub`. If the name is taken, Orbit adds `-` and the first 6
  characters of `sub`. The username is set once, at the first sign-in.
- **Synced fields.** Email and display name are copied from the token on every
  sign-in. They can't be edited in Orbit.
- **Roles with a role claim.** The user's roles are **replaced on every sign-in**
  from the claim (or the **Default role**). Role changes an admin makes in
  **Admin → Users** last only until the next sign-in. Manage roles in Entra.
- **Roles without a role claim.** The **Default role** is given once, at
  account creation. After that, admins manage roles in **Admin → Users**.
- **Owner** is not a role. SSO can hand out `admin`, but never owner rights.
- **Disabled accounts.** An account disabled in Orbit can't sign in with SSO
  ("Your account could not be provisioned (it may be disabled).").
- **Deleted accounts.** Deleting an SSO user in Orbit doesn't block them: their
  next SSO sign-in creates a fresh account with the **Default role**. Remove
  them in Entra (or the app assignment) as well.

> **`sub` is per app in Entra.** If you delete the app registration and create a
> new one, every user gets a new `sub`, and Orbit creates new accounts for
> them. Keep the registration; rotate only its secret.

## What SSO users can't do

- **No local password.** They can't set, change or reset one. Settings →
  **Password** says the identity provider manages it, and **Admin → Users** has
  no reset-password button for them.
- **No local two-factor.** They are exempt from **Enforce two-factor
  authentication**, and the two-factor section is hidden for them. Put MFA in
  Entra (Conditional Access).
- **Step-up** (when on) needs a password or a passkey. An SSO account has no
  password, so it confirms with a passkey. Add one in **Settings → Passkeys**
  (needs passkeys set up, see [CONFIGURATION.md](CONFIGURATION.md)).

## 8. (Optional) Make SSO the main way in

In **Owner → Security**:

| Option | Effect |
|---|---|
| **Hide password login** | The login page hides the username/password form while SSO is enabled. It only hides the form; the password API still works. |
| **Auto-start SSO sign-in** | Opening the login page goes straight to Entra. Needs SSO enabled (it's greyed out until then). |

Both are skipped at **`/login?local=1`**: the password form shows, and nothing
redirects. Keep a local owner account with a strong password and two-factor,
and use that address when Entra is down or misconfigured.

> Saving the SSO settings with SSO turned off also turns **Auto-start SSO
> sign-in** off.

Every SSO settings change is in the audit log (`auth.oidc_config`) and raises a
**Security/SSO config changed** alert if security alerts are on. Every SSO
sign-in is logged as `auth.oidc_login`.

## Troubleshooting

- **Test says "discovery failed"** → Check the issuer URL: tenant GUID, `/v2.0`
  at the end, no trailing slash. Check that the Orbit host can reach
  `login.microsoftonline.com` (proxy, firewall, DNS).
- **"The identity provider is unavailable."** → Orbit couldn't start the
  flow, usually because discovery failed. Click **Test** and read the error.
- **Entra shows `AADSTS50011` (redirect URI mismatch)** → The **Redirect URL**
  in Orbit and the **Web** redirect URI in Entra must be identical: `https`,
  host, path `/api/v1/auth/oidc/callback`, no trailing slash.
- **"The identity provider returned an error."** → Entra sent an error back
  (user cancelled, no consent, user not assigned when **Assignment required?**
  is on). The Entra sign-in logs show the reason.
- **"SSO sign-in failed."** → The token step failed. The server log line
  `oidc callback failed` has the cause. Usual ones: expired or wrong client
  secret (paste the **Value**, not the **Secret ID**); issuer mismatch
  (`/common` instead of the tenant ID); the login took over 10 minutes.
- **"Your account could not be provisioned (it may be disabled)."** → The
  account is disabled in Orbit, or the token had no `sub`. See the server
  log line `oidc provisioning failed`.
- **Signed in but sees nothing** → The user got the `none` role. Set a
  **Default role**, or check the role claim: decode an ID token (for example at
  jwt.ms) and compare its `roles` values with Orbit role names, letter for
  letter.
- **Role changes in Admin → Users keep reverting** → A **Role claim** is set, so
  roles come from Entra on every sign-in. Change them in Entra, or clear
  **Role claim**.
- **Locked out after Hide password login / Auto-start** → Open
  `https://orbit.example.org/login?local=1` and sign in with a local account.

More in [TROUBLESHOOTING.md](TROUBLESHOOTING.md) and [SECURITY.md](SECURITY.md).
