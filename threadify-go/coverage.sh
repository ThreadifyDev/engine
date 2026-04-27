#!/bin/bash

echo "Running engine coverage..."

# Get packages excluding archiver
PACKAGES=$(go list ./... | grep -v archiver)

# Remove existing coverage files
rm -f coverage.out coverage.html

# Run tests with coverage for each package individually
for pkg in $PACKAGES; do
    echo "Testing $pkg..."
    if go test -coverprofile=coverage_${pkg##*/}.out "$pkg" 2>/dev/null; then
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
        echo "Coverage report generated: coverage.html"
    else
        echo "No coverage data to generate report"
    fi
    
    # Clean up temporary files
    rm coverage_*.out
else
    echo "No coverage files generated - no tests found"
fi
