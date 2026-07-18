// Trait mapper for the "dex" OIDC provider (see ory/dex/config.yaml).
//
// dex's local-password connector issues an ID token with a stable set of
// standard OIDC claims (`sub`, `email`, `email_verified`, `name`) but does
// not guarantee a `preferred_username`/`nickname` claim the way some real
// providers do, so `username` falls back through the same candidate chain
// used by google.jsonnet/github.jsonnet: `preferred_username`, `nickname`,
// `given_name`, and finally the local part of `email`.
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
