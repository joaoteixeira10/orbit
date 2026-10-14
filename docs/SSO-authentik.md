# SSO with Authentik (OIDC)

This guide sets up single sign-on from **Authentik** to Orbit over OpenID
Connect. Use it when you run Authentik as your identity provider and want people
to sign in with their Authentik accounts, and optionally get their Orbit role
from Authentik groups. For Microsoft Entra ID, see [SSO-entra.md](SSO-entra.md).
How SSO fits with the rest of sign-in is in [AUTH-BLUEPRINT.md](AUTH-BLUEPRINT.md).

Replace these throughout:

| Placeholder | Meaning |
|---|---|
| `authentik.example.com` | your Authentik host |
| `orbit.example.org` | Orbit's public host name, as the browser sees it |
| `orbit` | the application **slug** you pick in step 3 |

> **The Orbit server calls Authentik itself.** Discovery, the code-for-token
> exchange and the key (JWKS) check all run server-side. The Orbit host must
> reach `authentik.example.com`, and that name must **not** resolve to a
> loopback or link-local address: Orbit refuses those on purpose (SSRF
> guard). Private LAN addresses (`10.x`, `192.168.x`, …) are fine.

## Before you start

- Only the **owner** can change SSO settings. The **SSO** tab is under
  **Owner**, not **Admin**.
- If the owner turned on **Step-up re-authentication** (**Owner → Owner →
  Feature flags**), saving SSO settings asks you to confirm with your password
  or a passkey first. The **Test** button does not.
- Orbit must run on its public host name with HTTPS. See
  [reverse-proxies.md](reverse-proxies.md).

## 1. (Optional) Plan the groups → roles mapping

Skip this if everyone should start with one fixed role. You set that as the
**Default role** in step 5.

Orbit matches the values of one ID-token claim to its **role names**. The
match is exact and case-sensitive. With Authentik, that claim is `groups`: a
list of group names.

Authentik's default `profile` scope mapping already sends `groups`, and Orbit
always asks for `profile`. So often you need nothing more than groups
named after Orbit roles.

> **Every group name that equals an Orbit role name grants that role.** A
> group called `admin` makes its members Orbit superusers. If your group
> names are shared with other apps, use a prefix and a custom mapping (below),
> so only `orbit-…` groups count.

To send only prefixed groups, with the prefix removed, create a custom scope
mapping. **Customization → Property Mappings → Create → Scope Mapping**:

- **Name**: `Orbit groups`
- **Scope name**: `orbit_groups`
- **Expression**:

  ```python
  return {
      "orbit_groups": [
          g.name.removeprefix("orbit-")
          for g in request.user.all_groups()
          if g.name.startswith("orbit-")
      ]
  }
  ```

A member of Authentik group `orbit-member` then gets `"orbit_groups":
["member"]`. You'll set **Extra scopes** and **Role claim** to `orbit_groups` in
step 5.

## 2. Create the OAuth2/OpenID provider

**Applications → Providers → Create → OAuth2/OpenID Provider**:

- **Name**: `orbit`
- **Authorization flow**: your default flow (explicit or implicit consent)
- **Client type**: **Confidential**
- **Client ID** and **Client Secret**: copy both, for step 5
- **Redirect URIs/Origins**: add exactly (strict match)

  ```
  https://orbit.example.org/api/v1/auth/oidc/callback
  ```

- **Signing Key**: an RSA key, for example the default
  `authentik Self-signed Certificate` (RS256). Don't leave it empty: an empty
  key makes Authentik sign with the client secret (HS256).
- **Advanced protocol settings → Scopes**: keep `openid`, `email`, `profile`,
  and add `Orbit groups` if you made it in step 1.
- **Include claims in id_token**: on. Orbit reads claims from the ID
  token, not from the userinfo endpoint.
- **Subject mode**: leave the default. Orbit keys accounts on `sub`, so
  changing it later creates new accounts for everyone.

## 3. Create the application

**Applications → Applications → Create**:

- **Name**: `Orbit`
- **Slug**: `orbit`
- **Provider**: the one from step 2

To limit who can sign in, bind groups or policies to the application
(**Policy / Group / User Bindings**).

## 4. Find the issuer URL

Authentik's issuer is per application, **with a trailing slash**:

```
https://authentik.example.com/application/o/orbit/
```

Check it in a browser:
`https://authentik.example.com/application/o/orbit/.well-known/openid-configuration`.
The `issuer` in that JSON must match what you enter in Orbit exactly.

## 5. Enter the settings in Orbit

Sign in as the owner and open **Owner → SSO** (**Single sign-on (OIDC)**).

| Field | Value |
|---|---|
| (toggle, top right) | on, to enable SSO |
| **Issuer URL** | `https://authentik.example.com/application/o/orbit/` |
| **Client ID** | from step 2 |
| **Client secret** | from step 2 |
| **Redirect URL** | `https://orbit.example.org/api/v1/auth/oidc/callback` |
| **Extra scopes** | `orbit_groups` with the custom mapping; else empty |
| **Role claim** | `orbit_groups` with the custom mapping, `groups` with plain group names, or empty |
| **Default role** | the role for new or unmatched users; empty means `none`. Use `member` if everyone should be able to create workspaces |

1. Click **Test** next to **Issuer URL**. "Discovery succeeded — issuer looks
   good" means Orbit reached Authentik and found a valid OIDC provider.
2. Click **Save SSO settings**.

Orbit always requests `openid profile email`; **Extra scopes** adds more,
separated by spaces. The **Client secret** is write-only. After saving, the
field shows `•••••••• (unchanged)`. Type into it only to replace the secret.

## 6. Create the matching roles

A claim value only grants access if an Orbit role has exactly that name.
The built-in roles are `admin`, `member` (creates workspaces) and `none`
(only workspaces they're invited to). For anything else, create the role in
**Admin → Roles** and give it instance privileges (for example `audit.read`
for an auditor). Roles never open workspaces: those come from being invited. See
[USER-GUIDE.md](USER-GUIDE.md).

How the claim is read:

- A list of strings is used as is; a single string is split on spaces.
- Values that match no Orbit role are ignored.
- No match at all → the **Default role**; if that's empty, the built-in
  **`none`** role (no access).

## 7. Sign in

Sign out. The login page now shows **Sign in with SSO**. Click it, sign in at
Authentik, and you land back in Orbit, signed in.

## How accounts and roles work

- **Just-in-time accounts.** The first SSO sign-in creates the Orbit
  account. It is keyed on the token's `sub`, never on the email.
- **Username.** From `preferred_username` (the Authentik username), else
  `email`, else `sub`. If the name is taken, Orbit adds `-` and the first 6
  characters of `sub`. The username is set once, at the first sign-in.
- **Synced fields.** Email and display name are copied from the token on every
  sign-in. They can't be edited in Orbit.
- **Roles with a role claim.** The user's roles are **replaced on every
  sign-in** from the claim (or the **Default role**). Role changes an admin makes
  in **Admin → Users** last only until the next sign-in. Manage groups in
  Authentik.
- **Roles without a role claim.** The **Default role** is given once, at
  account creation. After that, admins manage roles in **Admin → Users**.
- **Owner** is not a role. SSO can hand out `admin`, but never owner rights.
- **Disabled accounts.** An account disabled in Orbit can't sign in with SSO
  ("Your account could not be provisioned (it may be disabled).").
- **Deleted accounts.** Deleting an SSO user in Orbit doesn't block them: their
  next SSO sign-in creates a fresh account with the **Default role**. Deactivate
  them in Authentik (or remove the application binding) as well.

## What SSO users can't do

- **No local password.** They can't set, change or reset one. Settings →
  **Password** says the identity provider manages it, and **Admin → Users** has
  no reset-password button for them.
- **No local two-factor.** They are exempt from **Enforce two-factor
  authentication**, and the two-factor section is hidden for them. Put MFA in
  Authentik (an authenticator validation stage in the flow).
- **Step-up** (when on) needs a password or a passkey. An SSO account has no
  password, so it confirms with a passkey. Add one in **Settings → Passkeys**
  (needs passkeys set up, see [CONFIGURATION.md](CONFIGURATION.md)).

## 8. (Optional) Make SSO the main way in

In **Owner → Security**:

| Option | Effect |
|---|---|
| **Hide password login** | The login page hides the username/password form while SSO is enabled. It only hides the form; the password API still works. |
| **Auto-start SSO sign-in** | Opening the login page goes straight to Authentik. Needs SSO enabled (it's greyed out until then). |

Both are skipped at **`/login?local=1`**: the password form shows, and nothing
redirects. Keep a local owner account with a strong password and two-factor,
and use that address when Authentik is down or misconfigured.

> Saving the SSO settings with SSO turned off also turns **Auto-start SSO
> sign-in** off.

Every SSO settings change is in the audit log (`auth.oidc_config`) and raises a
**Security/SSO config changed** alert if security alerts are on. Every SSO
sign-in is logged as `auth.oidc_login`.

## Troubleshooting

- **Test says "discovery failed"** → Use the issuer exactly as Authentik
  reports it, trailing slash included. Check that the Orbit host can reach
  Authentik.
- **Test says "no allowed address for host"** → The Authentik host name
  resolves to a loopback or link-local address from the Orbit server (for
  example `localhost`, or a Docker setup that maps it to `127.0.0.1`). Use a
  name that resolves to a LAN or public address.
- **"The identity provider is unavailable."** → Orbit couldn't start the
  flow, usually because discovery failed. Click **Test** and read the error.
- **Authentik says the redirect URI is invalid** → The **Redirect URL** must be
  identical in Authentik and Orbit: `https`, host, path
  `/api/v1/auth/oidc/callback`, no trailing slash.
- **"The identity provider returned an error."** → Authentik sent an error back
  (user cancelled, or a policy denied access to the application). Check
  Authentik's **Events → Logs**.
- **"SSO sign-in failed."** → The token step failed. The server log line
  `oidc callback failed` has the cause. Usual ones: wrong client secret; issuer
  mismatch (missing trailing slash); no signing key on the provider; the login
  took over 10 minutes.
- **"Your account could not be provisioned (it may be disabled)."** → The
  account is disabled in Orbit, or the token had no `sub`. See the server
  log line `oidc provisioning failed`.
- **Signed in but sees nothing** → The user got the `none` role. Set a
  **Default role**, or check the role claim: the groups must be in the **ID
  token** (**Include claims in id_token** on), and their names must equal LB
  Admin role names, letter for letter.
- **Role changes in Admin → Users keep reverting** → A **Role claim** is set, so
  roles come from Authentik on every sign-in. Change the groups in Authentik,
  or clear **Role claim**.
- **Locked out after Hide password login / Auto-start** → Open
  `https://orbit.example.org/login?local=1` and sign in with a local account.

More in [TROUBLESHOOTING.md](TROUBLESHOOTING.md) and [SECURITY.md](SECURITY.md).
