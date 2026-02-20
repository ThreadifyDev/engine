# Changelog

All notable changes to the Threadify SDK will be documented in this file.

## [0.1.5] - 2026-02-04

### Added
- **CommonJS Support**: SDK now supports both ES Modules (ESM) and CommonJS (CJS)
  - Dual package exports in `package.json`
  - Automatic build script to generate CommonJS files
  - `dist/` folder with `.cjs` files for CommonJS users
  - Full backward compatibility with existing ESM code

### Changed
- Updated `package.json` with dual exports:
  - `"main": "dist/index.cjs"` - CommonJS entry point
  - `"module": "src/index.js"` - ES Module entry point
  - `"exports"` field for modern bundlers
- Added build script (`scripts/build-cjs.js`) to convert ESM to CJS
- Updated `prepublishOnly` script to build CJS files before publishing

### Documentation
- Added CommonJS usage examples in README
- Created `examples/commonjs-example.js` with full working example
- Added module support section explaining both ESM and CJS usage
- Updated Quick Start section with both ESM and CommonJS examples

### Files Added
- `scripts/build-cjs.js` - Build script for CommonJS conversion
- `examples/commonjs-example.js` - Complete CommonJS usage example
- `CHANGELOG.md` - This file

### Migration Guide
No breaking changes. Existing ESM code continues to work as before.

**For CommonJS users:**
```javascript
// Before (not supported)
const Threadify = require('@threadify/sdk'); // ❌ Error

// After (now supported)
const { Threadify } = require('@threadify/sdk'); // ✅ Works!
```

## [0.1.4] - 2026-02-04

### Changed
- Minor version bump

## [0.1.3] - Previous releases

See git history for earlier changes.
