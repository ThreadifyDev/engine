#!/bin/bash

echo "Running API coverage..."

# Change to API directory
cd api

# Remove existing coverage files
rm -f coverage.out coverage.html

# Run tests with coverage for all API packages
PACKAGES=$(go list ./...)

# Run tests for each API package
for pkg in $PACKAGES; do
    echo "Testing $pkg..."
    if go test -coverprofile=coverage_${pkg##*/}.out -coverpkg=./internal/service/...,./internal/handlers/... "$pkg" 2>/dev/null; then
        echo "  OK: $pkg"
    else
        echo "  Skipped: $pkg (no tests or failed)"
    fi
done

# Merge coverage files if any exist
if ls coverage_*.out 1> /dev/null 2>&1; then
    echo "Merging coverage files..."
    
    # Create a proper coverage.out file with mode header
    echo "mode: set" > coverage.out
    
    # Append coverage data from all files (skip mode lines)
    for file in coverage_*.out; do
        if [ -f "$file" ]; then
            grep -v "^mode:" "$file" >> coverage.out
        fi
    done
    
    # Generate HTML report if we have coverage data
    if [ -s coverage.out ] && [ "$(wc -l < coverage.out)" -gt 1 ]; then
        go tool cover -html=coverage.out -o coverage.html
        echo "Coverage report generated: api/coverage.html"
    else
        echo "No coverage data to generate report"
    fi
    
    # Clean up temporary files
    rm coverage_*.out
else
    echo "No coverage files generated - no tests found"
fi
