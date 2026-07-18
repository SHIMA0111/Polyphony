// Trait mapper for the "google" OIDC provider.
//
// Google's ID token reliably includes `email`/`email_verified`/`name`/
// `given_name`/`family_name`, but never a `preferred_username`/`nickname`
// claim (Google accounts have no public username), so `username` always
// falls back to `given_name` or the local part of `email`.
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
