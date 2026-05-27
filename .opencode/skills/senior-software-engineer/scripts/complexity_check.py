#!/usr/bin/env python3
"""
Complexity Check Script
A simple analyzer to warn about overly complex functions/files.
Usage: python scripts/complexity_check.py path/to/source_file.ts
"""

import sys
import os
import re

THRESHOLD_LINES = 50
THRESHOLD_IFS = 5

def analyze_file(filepath):
    with open(filepath, 'r') as f:
        lines = f.readlines()

    total_lines = len(lines)
    
    # Simple heuristics
    if_count = sum(1 for line in lines if "if (" in line or "if(" in line)
    loop_count = sum(1 for line in lines if "for (" in line or "while (" in line)
    
    print(f"📊 Analysis for: {filepath}")
    print(f"   Total Lines: {total_lines}")
    print(f"   'if' statements: {if_count}")
    print(f"   Loops: {loop_count}")

    issues = []
    
    if total_lines > 200:
        issues.append(f"❌ File is too long ({total_lines} lines). Consider splitting.")
        
    if if_count > THRESHOLD_IFS * 2:
        issues.append(f"⚠️ High branching complexity ({if_count} conditionals).")

    # Check for functions > 50 lines (Rough heuristic: indent level 0 or 1 function start to end)
    # This is hard to do accurately with regex, so we'll just check max indentation depth
    max_indent = 0
    for line in lines:
        stripped = line.lstrip()
        # Ignore comments
        if not stripped or stripped.startswith('//') or stripped.startswith('*'):
            continue
        # REACT/JSX EXCEPTION: Ignore lines starting with < or ending with > or /> 
        # (Layout nesting is not Logic nesting)
        if stripped.startswith('<') or stripped.endswith('>') or stripped.endswith('/>'):
            continue
        
        # Ignore closing syntax lines like "      )} " or "    ]" or "  });"
        if all(c in ")}];), " for c in stripped):
            continue
        
        if stripped.startswith(')') or stripped.startswith('}') or stripped.startswith(']'):
            continue
            
        indent = len(line) - len(stripped)
        if indent > max_indent:
            max_indent = indent
    
    
    # React often has deep visual nesting (Provider > Layout > Component > Map > Div)
    # So we allow 12 levels (24 spaces) for .tsx, vs 8 levels (16 spaces) for logic.
    limit = 24 if filepath.endswith('.tsx') else 16
    
    if max_indent > limit: 
        issues.append(f"❌ Excessive nesting detected (> {limit//2} levels). Refactor immediately.")

    if not issues:
        print("\n✅ Code Hygiene Check Passed.")
    else:
        print("\n🚨 Issues Found:")
        for issue in issues:
            print(issue)
        sys.exit(1)

if __name__ == "__main__":
    if len(sys.argv) < 2:
        print("Usage: python scripts/complexity_check.py <file>")
        sys.exit(1)
        
    analyze_file(sys.argv[1])
