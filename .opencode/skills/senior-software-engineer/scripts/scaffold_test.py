#!/usr/bin/env python3
"""
Scaffold Test Script
Generates a basic test file for a given input file.
Usage: python scripts/scaffold_test.py path/to/source_file.ts
"""

import sys
import os
from pathlib import Path

def generate_test_content(filename, name):
    return f"""import {{ describe, it, expect }} from 'vitest';
import {{ {name} }} from '../{filename}';

describe('{name}', () => {{
  it('should be defined', () => {{
    expect({name}).toBeDefined();
  }});

  it('should handle null inputs gracefully', () => {{
    // TODO: Implement null check test
    // expect({name}(null)).toBe(...);
  }});

  it('should return valid output schema', () => {{
    // TODO: Implement schema validation test
  }});
}});
"""

def main():
    if len(sys.argv) < 2:
        print("Usage: python scripts/scaffold_test.py <source_file>")
        sys.exit(1)

    source_path = Path(sys.argv[1])
    if not source_path.exists():
        print(f"Error: File {source_path} not found.")
        sys.exit(1)

    # Simple heuristic to guess the main export name based on filename
    # e.g., "lesson-plan-generator.ts" -> "lessonPlanGenerator"
    name_parts = source_path.stem.split('-')
    func_name = name_parts[0] + ''.join(x.title() for x in name_parts[1:])
    
    # Determine test path
    # src/ai/flows/foo.ts -> src/ai/flows/__tests__/foo.test.ts
    test_dir = source_path.parent / "__tests__"
    test_path = test_dir / f"{source_path.stem}.test.ts"

    if test_path.exists():
        print(f"Warning: Test file {test_path} already exists. Skipping.")
        sys.exit(0)

    # Create directory if needed
    test_dir.mkdir(exist_ok=True)

    # Write file
    content = generate_test_content(source_path.name, func_name)
    with open(test_path, 'w') as f:
        f.write(content)

    print(f"✅ Created test scaffold: {test_path}")
    print(f"   Target Function: {func_name}")

if __name__ == "__main__":
    main()
