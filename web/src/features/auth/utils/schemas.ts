import { z } from "zod"

/**
 * Client-side validation schema for {@link LoginFormValues}.
 *
 * Intentionally permissive on `password` (only a non-empty check) since the
 * login form only needs a non-empty value to submit to the API — the
 * server, not this schema, is the source of truth on whether the
 * email/password combination is actually correct.
 */
export const loginSchema = z.object({
  email: z
    .string()
    .min(1, "Email is required")
    .email("Enter a valid email address"),
  password: z.string().min(1, "Password is required"),
})

/** Inferred form values for {@link loginSchema}, used as the `useForm` generic in `LoginForm`. */
export type LoginFormValues = z.infer<typeof loginSchema>

/**
 * Client-side validation schema for {@link RegisterFormValues}.
 *
 * These are UX-layer rules only: `server/internal/interface/handler/auth_handler.go`
 * currently only rejects empty fields, so this schema is intentionally
 * stricter than (and not a claim of parity with) server-side validation.
 * The `.refine` at the object level cross-checks `password`/`confirmPassword`
 * and attaches its error to the `confirmPassword` field path so it renders
 * under that field's `Field.ErrorText`.
 */
export const registerSchema = z
  .object({
    email: z
      .string()
      .min(1, "Email is required")
      .email("Enter a valid email address"),
    username: z
      .string()
      .min(3, "Username must be at least 3 characters")
      .max(32, "Username must be at most 32 characters")
      .regex(
        /^[a-zA-Z0-9_]+$/,
        "Username may only contain letters, numbers, and underscores",
      ),
    password: z.string().min(8, "Password must be at least 8 characters"),
    confirmPassword: z.string().min(1, "Please confirm your password"),
  })
  .refine((data) => data.password === data.confirmPassword, {
    message: "Passwords do not match",
    path: ["confirmPassword"],
  })

/** Inferred form values for {@link registerSchema}, used as the `useForm` generic in `RegisterForm`. */
export type RegisterFormValues = z.infer<typeof registerSchema>
