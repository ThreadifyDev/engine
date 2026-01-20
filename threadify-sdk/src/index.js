import WebSocket from 'ws';
import { Connection, ThreadInstance } from './Thread.js';
import { Notification } from './Notification.js';
import { DataRetriever } from './DataRetriever.js';

/**
 * @typedef {Object} ThreadifyConnectOptions
 * @property {string} [url] - WebSocket URL (default: ws://localhost:8081/threads)
 * @property {string} [wsUrl] - WebSocket URL (alias for url)
 * @property {string} [graphqlUrl] - GraphQL URL (default: derived from wsUrl)
 * @property {boolean} [debug=false] - Enable debug logging
 * @property {number} [maxInFlight=10] - Maximum number of unACKed notifications (1-100)
 */

/**
 * Threadify SDK - Main entry point for connecting to Threadify Engine
 * @example
 * import { Threadify } from '@threadify/sdk';
 * 
 * // Basic connection
 * const connection = await Threadify.connect('api-key', 'my-service');
 * 
 * // Custom endpoints with debug
 * const connection = await Threadify.connect('api-key', 'my-service', {
 *   wsUrl: 'wss://api.example.com/threads',
 *   graphqlUrl: 'https://api.example.com/graphql',
 *   debug: true
 * });
 */
export class Threadify {
  /**
   * Connect to Threadify Engine
   * @param {string} apiKey - Your API key
   * @param {string} [serviceName] - Optional service name for identification
   * @param {ThreadifyConnectOptions} [options={}] - Connection options
   * @returns {Promise<import('./Thread.js').Connection>} Connected Connection instance
   * @throws {Error} When API key is invalid or missing
   * @example
   * const connection = await Threadify.connect('your-api-key', 'payment-service', {
   *   debug: true,
   *   wsUrl: 'wss://your-domain.com/threads'
   * });
   */
  static async connect(apiKey, serviceName = null, options = {}) {
    if (!apiKey || typeof apiKey !== 'string') {
      throw new Error('API key is required and must be a string');
    }

    const {
      url,
      wsUrl = url || 'wss://eng.threadify.dev/threads',
      graphqlUrl,
      debug = false,
      maxInFlight = 10
    } = options;

    // Validate maxInFlight
    if (maxInFlight < 1 || maxInFlight > 100) {
      throw new Error('maxInFlight must be between 1 and 100');
    }

    // Derive GraphQL URL from WebSocket URL if not provided
    const derivedGraphqlUrl = graphqlUrl || wsUrl
      .replace('ws://', 'http://')
      .replace('wss://', 'https://')
      .replace('/threads', '/graphql');

    return new Promise((resolve, reject) => {
      const ws = new WebSocket(wsUrl);
      // Initialize Connection with GraphQL URL, debug flag, and maxInFlight
      const connection = new Connection(ws, apiKey, serviceName, derivedGraphqlUrl, debug, maxInFlight);

      ws.on('open', () => {
        // Send connect message with maxInFlight
        const connectMessage = {
          action: 'connect',
          apiKey,
          serviceName,
          maxInFlight
        };

        connection._debugLog('WebSocket opened, sending connect message with maxInFlight:', maxInFlight);
        ws.send(JSON.stringify(connectMessage));
      });

      // Set up one-time handler for connect response only
      const connectHandler = (data) => {
        try {
          const message = JSON.parse(data.toString());
          connection._debugLog('Received message:', message.action);

          // Handle connect response
          if (message.action === 'connect') {
            ws.off('message', connectHandler); // Remove this handler after connect
            if (message.status === 'success') {
              connection.isConnected = true;
              connection._debugLog('Connection successful');
              resolve(connection);
            } else {
              reject(new Error(message.message || 'Connection failed'));
              ws.close();
            }
          }
        } catch (e) {
          connection._debugLog('Failed to parse WebSocket message:', e.message);
          if (debug) {
            console.error('[DEBUG] Raw data:', data.toString());
          }
        }
      };
      
      ws.on('message', connectHandler);

      ws.on('error', (error) => {
        reject(new Error(`WebSocket error: ${error.message}`));
      });

      ws.on('close', () => {
        connection.isConnected = false;
      });

      // Timeout after 10 seconds
      setTimeout(() => {
        if (!connection.isConnected) {
          reject(new Error('Connection timeout'));
          ws.close();
        }
      }, 10000);
    });
  }

  /**
   * Create a new Threadify instance with custom configuration
   * @param {Object} config - Configuration object
   * @param {string} config.apiKey - Your API key
   * @param {string} config.url - WebSocket URL
   * @param {string} config.wsUrl - WebSocket URL (alias for url)
   * @param {string} config.graphqlUrl - GraphQL URL
   * @param {string} config.serviceName - Service name
   * @returns {Object} - Threadify instance with connect method
   */
  static create(config) {
    return {
      connect: (serviceName = config.serviceName) => {
        return Threadify.connect(config.apiKey, serviceName, {
          url: config.url,
          wsUrl: config.wsUrl,
          graphqlUrl: config.graphqlUrl
        });
      }
    };
  }
}

// Export for CommonJS compatibility
export default Threadify;

// Export classes for direct usage (DataRetriever is now internal)
export { Connection, ThreadInstance, Notification };
