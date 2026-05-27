# 20-Point Code Review Checklist

Usage: Run through this list before marking any task as "Done".

## I. Correctness & Reliability
1. [ ] **Logic:** Does the code actually do what the requirements say?
2. [ ] **Edge Cases:** Are `null`, `undefined`, empty arrays, and negative numbers handled?
3. [ ] **Error Handling:** Are errors caught? Are they meaningful? (No `console.log(error)`)
4. [ ] **Concurrency:** Are `await` calls necessary? Are there race conditions?
5. [ ] ** Determinism:** Is GenAI output validated by a Zod schema?

## II. Type Safety (TypeScript)
6. [ ] **No `any`:** Are there any `any` types? (Strictly forbidden).
7. [ ] **Return Types:** Do exported functions have explicit return types?
8. [ ] **Strictness:** Are optional parameters (`?`) actually handled?
9. [ ] **Generics:** Are generics used appropriately to avoid repetition?

## III. Security
10. [ ] **Inputs:** Is user input validated/sanitized?
11. [ ] **Secrets:** Are there any hardcoded keys/passwords? (Check `.env` usage).
12. [ ] **Authorization:** Does the user have permission to perform this action?

## IV. Performance
13. [ ] **loops:** Are there O(n^2) nested loops on potentially large datasets?
14. [ ] **Re-renders:** (React) Are `useMemo` / `useCallback` used where props change often?
15. [ ] **I/O:** Are database/API calls batched where possible?

## V. Maintainability
16. [ ] **Naming:** Do variables reveal intent? (`isLoading` vs `flag`)
17. [ ] **Complexity:** Is any single function > 50 lines?
18. [ ] **DRY:** Is code duplicated?
19. [ ] **Comments:** Do comments explain *why*, not *what*?
20. [ ] **Tests:** Is there a corresponding unit test or manual verification step?
