// Trait mapper for the "google" OIDC provider.
//
// Google's ID token reliably includes `email`/`email_verified`/`name`/
// `given_name`/`family_name`, but never a `preferred_username`/`nickname`
// claim (Google accounts have no public username), so `username` always
// falls back to `given_name` or the local part of `email`.
//
// Output must satisfy ory/kratos/identity.schema.json's `traits.email`
// (required, format: email) and `traits.username` (required, 3-100 chars).
// Each candidate in the fallback chain is accepted only if it clears the
// 3-char minLength; the value ultimately chosen is then clamped to the
// 100-char maxLength via std.substr before being returned.
local claims = std.extVar('claims');

local emailLocalPart = std.split(claims.email, '@')[0];

local candidate =
  if std.objectHas(claims, 'preferred_username') && std.length(claims.preferred_username) >= 3 then
    claims.preferred_username
  else if std.objectHas(claims, 'nickname') && std.length(claims.nickname) >= 3 then
    claims.nickname
  else if std.objectHas(claims, 'given_name') && std.length(claims.given_name) >= 3 then
    claims.given_name
  else
    // Terminal fallback: the local part of the email address, which is not
    // itself length-checked. If it is still under 3 chars (e.g. "ab" from
    // "ab@example.com"), the schema's minLength:3 rejects the identity
    // outright at creation time; that failure mode is accepted here as a
    // dev-mock edge case rather than worked around further.
    emailLocalPart;

local username = std.substr(candidate, 0, 100);

{
  identity: {
    traits: {
      email: claims.email,
      username: username,
    },
  },
}
