import WebSocket from 'ws';
import { Connection, ThreadInstance } from './Thread.js';

/**
 * Threadify SDK - Main entry point
 */
export class Threadify {
  /**
   * Connect to Threadify Engine
   * @param {string} apiKey - Your API key
   * @param {string} serviceName - Optional service name for identification
   * @param {Object} options - Connection options
   * @param {string} options.url - WebSocket URL (default: ws://localhost:8081/threads)
   * @returns {Promise<Connection>} - Connected Connection instance
   */
  static async connect(apiKey, serviceName = null, options = {}) {
    if (!apiKey || typeof apiKey !== 'string') {
      throw new Error('API key is required and must be a string');
    }

    const {
      url = 'ws://localhost:8081/threads'
    } = options;
    console.log('[DEBUG] Connecting to Threadify Engine at:', url);

    return new Promise((resolve, reject) => {
      const ws = new WebSocket(url);
      // Initialize Connection
      const connection = new Connection(ws, apiKey, serviceName);

      ws.on('open', () => {
        console.log('[DEBUG] WebSocket opened');
        // Send connect message
        const connectMessage = {
          action: 'connect',
          apiKey,
          serviceName
        };

        console.log('[DEBUG] Sending connect message:', JSON.stringify(connectMessage));
        ws.send(JSON.stringify(connectMessage));
      });

      ws.on('message', (data) => {
        console.log('[DEBUG] Received message:', data.toString());
        try {
          const message = JSON.parse(data.toString());
          console.log('[DEBUG] Parsed message:', message);

          // Handle connect response
          if (message.action === 'connect') {
            console.log('[DEBUG] Connect response received, status:', message.status);
            if (message.status === 'success') {
              connection.isConnected = true;
              console.log('[DEBUG] Connection successful');
              resolve(connection);
            } else {
              reject(new Error(message.message || 'Connection failed'));
              ws.close();
            }
          }  // Don't process further for connect messages

          // Handle event notifications
          if (message.action in connection.eventHandlers) {
            connection.eventHandlers[message.action].forEach(handler => {
              try {
                handler(message);
              } catch (e) {
                console.error('Error in event handler:', e);
              }
            });
          }
        } catch (e) {
          console.error('[DEBUG] Failed to parse WebSocket message:', e);
          console.error('[DEBUG] Raw data:', data.toString());
        }
      });

      ws.on('error', (error) => {
        reject(new Error(`WebSocket error: ${error.message}`));
      });

      ws.on('close', () => {
        thread.isConnected = false;
      });

      // Timeout after 10 seconds
      setTimeout(() => {
        if (!thread.isConnected) {
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
   * @param {string} config.serviceName - Service name
   * @returns {Object} - Threadify instance with connect method
   */
  static create(config) {
    return {
      connect: (serviceName = config.serviceName) => {
        return Threadify.connect(config.apiKey, serviceName, {
          url: config.url
        });
      }
    };
  }
}

// Export for CommonJS compatibility
export default Threadify;

// Export Connection class for direct usage
export { Connection, ThreadInstance };
