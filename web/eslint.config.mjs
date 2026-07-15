import nextCoreWebVitals from "eslint-config-next/core-web-vitals";
import nextTypescript from "eslint-config-next/typescript";

const eslintConfig = [
  ...nextCoreWebVitals,
  ...nextTypescript,
  {
    rules: {
      // The codebase does not yet use a data-fetching library (React Query/SWR);
      // fetch-on-mount via useEffect + setState is the intentional pattern used
      // throughout features/*. Downgrade to a warning instead of rewriting the
      // fetch architecture as part of an unrelated infra lint-tooling fix.
      "react-hooks/set-state-in-effect": "warn",
    },
  },
  {
    ignores: [
      "node_modules/**",
      ".next/**",
      "out/**",
      "build/**",
      "next-env.d.ts",
    ],
  },
];

export default eslintConfig;
