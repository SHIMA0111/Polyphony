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
local claims = std.extVar('claims');

local emailLocalPart = std.split(claims.email, '@')[0];

local username =
  if std.objectHas(claims, 'preferred_username') && claims.preferred_username != '' then
    claims.preferred_username
  else if std.objectHas(claims, 'nickname') && claims.nickname != '' then
    claims.nickname
  else if std.objectHas(claims, 'given_name') && claims.given_name != '' then
    claims.given_name
  else
    emailLocalPart;

{
  identity: {
    traits: {
      email: claims.email,
      username: username,
    },
  },
}
