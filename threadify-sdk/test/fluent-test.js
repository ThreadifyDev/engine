import { Threadify } from '../src/index.js';

async function testFluentAPI() {
  console.log('🚀 Testing Fluent API with New Authentication Flow...\n');

  try {
    // Initialize Threadify with valid API key from our mock auth service
    console.log('📡 Connecting to Threadify...');
    const connection = await Threadify.connect('api-key-123', 'payment-service');
    console.log('✅ Connected successfully!\n');

    // Start thread without contract (non-contract flow)
    console.log('🏁 Starting thread WITHOUT contract...');
    const startStartTime = Date.now();
    const threadInstance = await connection.start();
    const startEndTime = Date.now();
    console.log(`✅ Thread started in ${startEndTime - startStartTime}ms\n`);

    // Test fluent API with non-contract workflow
    console.log('🔄 Testing fluent step API...');
    const step1StartTime = Date.now();
    await threadInstance
      .step('data-processing')
      .addContext({ 
        workflowType: 'custom-workflow',
        stepName: 'data-processing',
        environment: 'test',
        version: '1.0',
        company: 'company-abc',
        user: 'user-123'
      })
      .stop('success', 'Data processed successfully', { 
        recordsProcessed: '100',
        result: 'passed' 
      });
    const step1EndTime = Date.now();
    console.log(`✅ Step completed in ${step1EndTime - step1StartTime}ms\n`);

    // Test retry flow
    console.log('🔄 Testing retry flow...');
    await threadInstance
      .step('payment-processing')
      .addContext({ 
        amount: 100,
        currency: 'USD',
        company: 'company-abc'
      })
      .stop('failed', 'Payment failed - will retry', { 
        error: 'payment_gateway_timeout',
        retryCount: 1
      });

    // Retry the same step with success
    await threadInstance
      .step('payment-processing')
      .addContext({ 
        amount: 100,
        currency: 'USD',
        retryAttempt: 2
      })
      .stop('success', 'Payment successful on retry', { 
        transactionId: 'txn_12345',
        result: 'passed'
      });
    
    console.log('✅ Retry flow completed\n');

    // Test custom workflow steps
    console.log('🎯 Testing custom workflow steps...');
    await threadInstance
      .step('notification')
      .addContext({ 
        recipients: ['user@example.com'],
        type: 'email_notification'
      })
      .stop('success', 'Notifications sent', { 
        sentCount: 1,
        result: 'passed'
      });

    console.log('✅ Custom workflow completed\n');

    // Close connection
    console.log('🔌 Closing connection...');
    const closeStartTime = Date.now();
    await threadInstance.close();
    const closeEndTime = Date.now();
    console.log(`✅ Closed in ${closeEndTime - closeStartTime}ms\n`);

    console.log('🎉 Non-contract tests passed!');
    console.log('✅ API key authentication');
    console.log('✅ Company-based permissions');
    console.log('✅ Non-contract workflow');
    console.log('✅ Fluent API and retry mechanisms\n');

    // ============================================
    // CONTRACT-BASED WORKFLOW TEST
    // ============================================
    console.log('📄 Testing CONTRACT-BASED workflow...\n');

    // Start a contract-based thread with role
    console.log('🏁 Starting contract thread with buyer role...');
    const contractThread = await connection.start('purchase_order:1', 'buyer-service');
    console.log(`✅ Contract thread started: ${contractThread.threadId}`);
    console.log(`📄 Contract: ${contractThread.getContractId()}\n`);

    // Execute step with buyer role
    console.log('📝 Executing create_order step (requires buyer role)...');
    await contractThread
      .step('create_order')
      .addContext({ 
        orderId: 'PO-12345',
        items: 'Widget x100',
        totalAmount: '2500.00',
        currency: 'USD',
        buyerCompany: 'Acme Corp',
        timestamp: new Date().toISOString()
      })
      .stop('success', 'Purchase order created', { 
        orderId: 'PO-12345',
        status: 'pending'
      });


    const orderStep = await contractThread.step('create_order')
    // BUSINESS LOGIC
    orderStep.addContext({"newInfo": "INformation"})
    // BUSINESS LOGIC
    orderStep.addContext({"extraNewInfo": "INformation"})
    // BUSINESS LOGIC

    orderStep.stop()
    console.log('✅ Order created successfully\n');

    // Create invitation for seller
    console.log('📨 Creating invitation for seller with seller role...');
    const sellerToken = await contractThread.inviteParty({
      role: 'seller',
      permissions: 'read,write',
      expiresIn: '7d'
    });
    console.log(`✅ Seller invitation created: ${sellerToken.substring(0, 30)}...\n`);

    // Simulate seller joining (in real scenario, this would be a different service)
    console.log('🤝 Seller joining thread...');
    const sellerConnection = await Threadify.connect('seller-api-key', 'seller-service');
    const sellerThread = await sellerConnection.join(sellerToken);
    console.log(`✅ Seller joined: ${sellerThread.threadId}`);
    console.log(`👤 Role: ${sellerThread.role}`);
    console.log(`🔐 Permissions: ${sellerThread.permissions}\n`);

    // Seller executes their step
    console.log('✅ Seller confirming order (requires seller role)...');
    await sellerThread
      .step('confirm_order')
      .addContext({ 
        orderId: 'PO-12345',
        confirmationNumber: 'CONF-98765',
        estimatedDelivery: '2025-01-15'
      })
      .stop('success', 'Order confirmed', { 
        confirmationNumber: 'CONF-98765',
        status: 'confirmed'
      });
    console.log('✅ Order confirmed by seller\n');

    // Close contract thread connections
    await contractThread.close();
    await sellerThread.close();
    console.log('✅ Contract workflow completed\n');

    console.log('🎉 ALL TESTS PASSED!');
    console.log('✅ Non-contract workflows');
    console.log('✅ Contract-based workflows with roles');
    console.log('✅ Cross-party collaboration via invitations');
    console.log('✅ Role-based step execution');

  } catch (error) {
    console.error('❌ Error:', error.message);
    
    if (error.message.includes('API key') || error.message.includes('authentication')) {
      console.error('🔐 Authentication failed - check API key');
    } else if (error.message.includes('role')) {
      console.error('🔐 Role validation failed - check contract role requirements');
    } else if (error.message.includes('permission')) {
      console.error('🔐 Permission denied - check user permissions');
    }
  }
}

testFluentAPI();
