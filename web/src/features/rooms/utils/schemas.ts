import { z } from "zod"

/**
 * Client-side validation schema for {@link CreateRoomFormValues}, used by
 * `CreateRoomForm`'s `useForm` + `zodResolver`.
 *
 * `description` is optional and defaults to an empty string so the inferred
 * form values always carry a `string` (never `undefined`), matching the
 * shape `useCreateRoom`'s mutation function expects.
 */
export const createRoomSchema = z.object({
  name: z
    .string()
    .trim()
    .min(1, "Room name is required")
    .max(255, "Room name must be at most 255 characters"),
  description: z
    .string()
    .trim()
    .max(2000, "Description must be at most 2000 characters")
    .optional()
    .default(""),
})

/**
 * Pre-parse form values for {@link createRoomSchema} (`description` is
 * optional/`undefined` before the default applies) — the shape
 * `react-hook-form` tracks field state as, and the `TFieldValues` generic
 * for `useForm` in `CreateRoomForm`.
 */
export type CreateRoomFormValues = z.input<typeof createRoomSchema>

/**
 * Post-parse (validated + defaulted) values for {@link createRoomSchema} —
 * the shape `handleSubmit`'s callback actually receives once `zodResolver`
 * has run, and what's passed to `useCreateRoom`'s mutation function.
 */
export type CreateRoomFormOutput = z.output<typeof createRoomSchema>
