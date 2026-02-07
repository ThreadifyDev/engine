#!/usr/bin/env node

/**
 * Build script to generate CommonJS version of the SDK
 * Converts ESM imports to CommonJS require() statements
 */

import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

const srcDir = path.join(__dirname, '..', 'src');
const distDir = path.join(__dirname, '..', 'dist');

// Ensure dist directory exists
if (!fs.existsSync(distDir)) {
  fs.mkdirSync(distDir, { recursive: true });
}

/**
 * Convert ESM to CommonJS
 */
function convertToCommonJS(content, filename) {
  let converted = content;

  // Convert import statements
  // import WebSocket from 'ws' -> const WebSocket = require('ws')
  converted = converted.replace(
    /import\s+(\w+)\s+from\s+['"]([^'"]+)['"]/g,
    'const $1 = require(\'$2\')'
  );

  // import { A, B } from 'module' -> const { A, B } = require('module')
  converted = converted.replace(
    /import\s+\{([^}]+)\}\s+from\s+['"]([^'"]+)['"]/g,
    'const {$1} = require(\'$2\')'
  );

  // Convert local imports to .cjs extension
  converted = converted.replace(
    /require\(['"]\.\/([^'"]+)\.js['"]\)/g,
    'require(\'./$1.cjs\')'
  );

  // Convert export statements
  // export class X -> module.exports.X = class X
  // export { A, B } -> module.exports = { A, B }
  // export default X -> module.exports = X

  // For index.js, we need special handling - do this FIRST
  if (filename === 'index.js') {
    // Remove all export statements - we'll handle them manually
    converted = converted.replace(/export\s+default\s+\w+;?\s*/g, '');
    converted = converted.replace(/export\s+\{[^}]+\};?\s*/g, '');
    converted = converted.replace(/export\s+(class|function)\s+(\w+)/g, '$1 $2');

    // Add proper CommonJS exports at the end
    // Export as an object to support both:
    // - const { Threadify, Connection } = require('@threadify/sdk')
    // - const Threadify = require('@threadify/sdk').default
    // - const sdk = require('@threadify/sdk'); sdk.Threadify.connect(...)
    converted += `\n
// CommonJS exports
module.exports = {
  Threadify,
  Connection,
  ThreadInstance,
  Notification,
  default: Threadify
};
`;
  } else {
    // For other files, handle exports normally
    
    // Collect exported class/function names
    const exportedNames = [];
    const classMatches = content.match(/export\s+(class|function)\s+(\w+)/g);
    if (classMatches) {
      classMatches.forEach(match => {
        const name = match.match(/export\s+(?:class|function)\s+(\w+)/)[1];
        exportedNames.push(name);
      });
    }
    
    // Handle: export class/function Name - just remove export keyword
    converted = converted.replace(
      /export\s+(class|function)\s+(\w+)/g,
      '$1 $2'
    );

    // Handle: export { A, B, C }
    const exportMatches = converted.match(/export\s+\{([^}]+)\}/g);
    if (exportMatches) {
      exportMatches.forEach(match => {
        const exports = match.match(/\{([^}]+)\}/)[1].trim();
        converted = converted.replace(match, '');
        // Add exported names to the list
        exports.split(',').forEach(name => {
          exportedNames.push(name.trim());
        });
      });
    }

    // Handle: export default X
    const defaultMatch = converted.match(/export\s+default\s+(\w+)/);
    if (defaultMatch) {
      converted = converted.replace(
        /export\s+default\s+(\w+)/g,
        ''
      );
      exportedNames.push('default: ' + defaultMatch[1]);
    }

    // Add module.exports at the end with all collected exports
    if (exportedNames.length > 0) {
      converted += `\n\nmodule.exports = { ${exportedNames.join(', ')} };\n`;
    }
  }

  return converted;
}

/**
 * Process all JS files in src directory
 */
function buildCJS() {
  console.log('🔨 Building CommonJS version...');

  const files = fs.readdirSync(srcDir).filter(f => f.endsWith('.js'));

  files.forEach(file => {
    const srcPath = path.join(srcDir, file);
    const distPath = path.join(distDir, file.replace('.js', '.cjs'));

    console.log(`   Converting ${file} -> ${path.basename(distPath)}`);

    const content = fs.readFileSync(srcPath, 'utf-8');
    const converted = convertToCommonJS(content, file);

    fs.writeFileSync(distPath, converted, 'utf-8');
  });

  console.log('✅ CommonJS build complete!');
  console.log(`   Output: ${distDir}/`);
}

// Run the build
buildCJS();
