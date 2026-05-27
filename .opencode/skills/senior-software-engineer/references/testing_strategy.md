# Testing Strategy Guide

## Philosophy: The Testing Pyramid
We strictly adhere to the Pyramid model. We do not write E2E tests for everything.

### 1. Unit Tests (70%)
*   **What:** Test individual functions, classes, and logic blocks in isolation.
*   **Where:** `src/tests/unit/` or co-located `__tests__`.
*   **Mocking:** Heavy mocking of external dependencies (Databases, APIs, GenAI).
*   **Speed:** Must run in < 10ms per test.

### 2. Integration Tests (20%)
*   **What:** Test the interaction between two modules (e.g., Service Layer + Database).
*   **Where:** `src/tests/integration/`.
*   **Mocking:** Minimal. Use Test Containers or local emulators.
*   **Speed:** < 500ms per test.

### 3. E2E / AI Flow Tests (10%)
*   **What:** Test the full user journey or full AI pipeline.
*   **Where:** `src/tests/e2e/`.
*   **Mocking:** None. Use real (sandbox) environment.
*   **Cost:** Expensive. Run only on pre-push or CI.

## Testing GenAI Code
Testing probabilistic code requires specific strategies:

1.  **Deterministic Scaffolding:**
    *   Test the *logic around* the AI, not the AI itself.
    *   Mock the AI response to test:
        *   Schema validation success.
        *   Schema validation failure (malformed JSON).
        *   Network timeout.
    
2.  **Golden Datasets:**
    *   Maintain a set of "Correct" inputs and "Acceptable" outputs.
    *   Use cosine similarity checks (if available) or keyword presence matching.

3.  **Snapshot Testing:**
    *   Use snapshots for prompt templates to ensure no accidental prompt drift.
