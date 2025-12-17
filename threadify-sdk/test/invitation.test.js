import { Thread } from '../src/index.js';
import { WebSocket } from 'ws';

// Mock WebSocket for testing
jest.mock('ws');

describe('Thread Invitation SDK', () => {
  let mockWs;
  let thread;

  beforeEach(() => {
    mockWs = {
      send: jest.fn(),
      on: jest.fn(),
      close: jest.fn(),
      readyState: WebSocket.OPEN
    };
    
    // Mock WebSocket constructor
    WebSocket.mockImplementation(() => mockWs);
    
    // Create thread instance
    thread = new Thread(mockWs, 'test-api-key', 'test-owner');
    thread.isConnected = true;
    thread.threadId = 'test-thread-123';
    thread.contractId = 'test-contract-456';
  });

  describe('inviteParty', () => {
    test('should create invitation token with required role only', async () => {
      const mockResponse = {
        action: 'inviteParty',
        status: 'success',
        threadToken: 'jwt-token-123'
      };

      // Mock the _onceResponse method
      thread._onceResponse = jest.fn((callback) => {
        callback(mockResponse);
      });
      
      thread._send = jest.fn();

      const token = await thread.inviteParty({ role: 'external_partner' });

      expect(token).toBe('jwt-token-123');
      expect(thread._send).toHaveBeenCalledWith({
        action: 'inviteParty',
        role: 'external_partner',
        permissions: 'read,write',
        expiresIn: '24h'
      });
    });

    test('should create invitation token with custom permissions and expiry', async () => {
      const mockResponse = {
        action: 'inviteParty',
        status: 'success',
        threadToken: 'jwt-token-456'
      };

      thread._onceResponse = jest.fn((callback) => {
        callback(mockResponse);
      });
      
      thread._send = jest.fn();

      const token = await thread.inviteParty({
        role: 'contractor',
        permissions: 'read',
        expiresIn: '7d'
      });

      expect(token).toBe('jwt-token-456');
      expect(thread._send).toHaveBeenCalledWith({
        action: 'inviteParty',
        role: 'contractor',
        permissions: 'read',
        expiresIn: '7d'
      });
    });

    test('should throw error when role is missing', async () => {
      await expect(thread.inviteParty({})).rejects.toThrow('Role is required for inviteParty');
    });

    test('should throw error when thread is not connected', async () => {
      thread.isConnected = false;
      
      await expect(thread.inviteParty({ role: 'external_partner' }))
        .rejects.toThrow('Thread must be connected and started to create invitations');
    });

    test('should throw error when threadId is missing', async () => {
      thread.threadId = null;
      
      await expect(thread.inviteParty({ role: 'external_partner' }))
        .rejects.toThrow('Thread must be connected and started to create invitations');
    });

    test('should handle server error response', async () => {
      const mockResponse = {
        action: 'inviteParty',
        status: 'error',
        message: 'Invalid role'
      };

      thread._onceResponse = jest.fn((callback) => {
        callback(mockResponse);
      });
      
      thread._send = jest.fn();

      await expect(thread.inviteParty({ role: 'invalid_role' }))
        .rejects.toThrow('Invalid role');
    });
  });

  describe('Thread.join (static)', () => {
    test('should join thread with valid token', async () => {
      const mockWs = {
        send: jest.fn(),
        on: jest.fn((event, callback) => {
          if (event === 'open') {
            setTimeout(callback, 10);
          } else if (event === 'message') {
            // Simulate connect response
            setTimeout(() => callback(JSON.stringify({
              action: 'connect',
              status: 'success'
            })), 20);
            // Simulate joinThread response
            setTimeout(() => callback(JSON.stringify({
              action: 'joinThread',
              status: 'success',
              threadId: 'joined-thread-789',
              contractId: 'contract-456',
              role: 'external_partner',
              permissions: 'read,write'
            })), 30);
          }
        }),
        close: jest.fn(),
        readyState: WebSocket.OPEN
      };

      WebSocket.mockImplementation(() => mockWs);

      const joinedThread = await Thread.join('jwt-token-123', 'api-key', 'user-123');

      expect(joinedThread).toBeInstanceOf(Thread);
      expect(joinedThread.threadId).toBe('joined-thread-789');
      expect(joinedThread.contractId).toBe('contract-456');
      expect(joinedThread.role).toBe('external_partner');
      expect(joinedThread.permissions).toBe('read,write');
      expect(joinedThread.isConnected).toBe(true);
    });

    test('should throw error when token is missing', async () => {
      await expect(Thread.join('', 'api-key', 'user-123'))
        .rejects.toThrow('Thread token is required for join');
    });

    test('should throw error when API key is missing', async () => {
      await expect(Thread.join('token', '', 'user-123'))
        .rejects.toThrow('API key is required for join');
    });

    test('should throw error when owner ID is missing', async () => {
      await expect(Thread.join('token', 'api-key', ''))
        .rejects.toThrow('Owner ID is required for join');
    });

    test('should handle connection failure', async () => {
      const mockWs = {
        send: jest.fn(),
        on: jest.fn((event, callback) => {
          if (event === 'open') {
            setTimeout(callback, 10);
          } else if (event === 'message') {
            setTimeout(() => callback(JSON.stringify({
              action: 'connect',
              status: 'error',
              message: 'Invalid API key'
            })), 20);
          }
        }),
        close: jest.fn(),
        readyState: WebSocket.OPEN
      };

      WebSocket.mockImplementation(() => mockWs);

      await expect(Thread.join('token', 'invalid-key', 'user-123'))
        .rejects.toThrow('Invalid API key');
    });

    test('should handle join failure', async () => {
      const mockWs = {
        send: jest.fn(),
        on: jest.fn((event, callback) => {
          if (event === 'open') {
            setTimeout(callback, 10);
          } else if (event === 'message') {
            setTimeout(() => callback(JSON.stringify({
              action: 'connect',
              status: 'success'
            })), 20);
            setTimeout(() => callback(JSON.stringify({
              action: 'joinThread',
              status: 'error',
              message: 'Invalid token'
            })), 30);
          }
        }),
        close: jest.fn(),
        readyState: WebSocket.OPEN
      };

      WebSocket.mockImplementation(() => mockWs);

      await expect(Thread.join('invalid-token', 'api-key', 'user-123'))
        .rejects.toThrow('Invalid token');
    });
  });
});
