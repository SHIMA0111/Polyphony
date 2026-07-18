// Trait mapper for the "github" OIDC provider.
//
// Kratos's built-in GitHub provider adapter maps GitHub's user API `login`
// field into the `preferred_username` claim, so this is normally populated —
// but GitHub accounts may still omit a public email (`email` can be null if
// the user hides it and has no public email set), which the identity
// schema requires; `nickname`/`given_name` are included as defensive
// fallbacks for `username` in case Kratos's mapping ever changes, matching
// dex.jsonnet/google.jsonnet's shared shape.
//
// Output must satisfy ory/kratos/identity.schema.json's `traits.email`
// (required, format: email) and `traits.username` (required, 3-100 chars).
// Each fallback candidate is only accepted once it clears the schema's
// 3-char minimum -- a 1-2 char `given_name`/`nickname` would otherwise
// pass the "non-empty" check below but still fail identity creation.
local claims = std.extVar('claims');

local emailLocalPart = std.split(claims.email, '@')[0];

// The email-local-part fallback is the terminal candidate: if it is also
// under 3 chars (e.g. `a@example.com`), the resulting username still fails
// the schema's minLength and Kratos will reject the identity. Acceptable
// for this dev-mock provider chain; a real deployment would need a
// disambiguating suffix or a dedicated username-collection step instead.
local username =
  if std.objectHas(claims, 'preferred_username') && std.length(claims.preferred_username) >= 3 then
    claims.preferred_username
  else if std.objectHas(claims, 'nickname') && std.length(claims.nickname) >= 3 then
    claims.nickname
  else if std.objectHas(claims, 'given_name') && std.length(claims.given_name) >= 3 then
    claims.given_name
  else
    emailLocalPart;

{
  identity: {
    traits: {
      email: claims.email,
      username: std.substr(username, 0, 100),
    },
  },
}
