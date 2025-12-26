import { Threadify, Thread } from '../src/index.js';

/**
 * Example: Thread Invitation System
 * 
 * This example demonstrates:
 * 1. Creating a thread and inviting users
 * 2. Joining a thread with an invitation token
 * 3. Using the invitation system for collaboration
 */

async function invitationExample() {
  console.log('🚀 Threadify Invitation System Example\n');

  try {
    // === HOST: Create thread and invite participants ===
    console.log('📋 HOST: Creating thread...');
    
    const hostThread = await Threadify.connect('test-api-key', 'host-service');
    console.log('✅ Host connected successfully');

    // Start a new thread (without contract)
    const startedThread = await hostThread.start('payment_flow', 'merchant', {
      refs: {
        customer_id: '12345',
        order_id: 'order-67890'
      }
    });
    console.log(`✅ Thread created: ${startedThread.threadId}`);

    // Create invitations for different roles
    console.log('\n📨 Creating invitations...');
    
    // Simple invitation (role only)
    const contractorToken = await hostThread.inviteParty({
      role: 'contractor'
    });
    console.log(`✅ Contractor invitation created: ${contractorToken.substring(0, 20)}...`);

    // Custom permissions
    const auditorToken = await hostThread.inviteParty({
      role: 'auditor',
      permissions: 'read'
    });
    console.log(`✅ Auditor invitation created: ${auditorToken.substring(0, 20)}...`);

    // Full configuration
    const partnerToken = await hostThread.inviteParty({
      role: 'external_partner',
      permissions: 'read,write,execute',
      expiresIn: '7d'
    });
    console.log(`✅ Partner invitation created: ${partnerToken.substring(0, 20)}...`);

    // === GUEST: Join thread with invitation ===
    console.log('\n👥 GUEST: Joining thread with invitation...');
    
    const guestThread = await Threadify.connect('test-api-key', 'guest-service');
    await guestThread.join(contractorToken);
    
    console.log(`✅ Guest joined thread: ${guestThread.threadId}`);
    console.log(`📄 Contract: ${guestThread.contractId}`);
    console.log(`👤 Role: ${guestThread.role}`);
    console.log(`🔐 Permissions: ${guestThread.permissions}`);

    // === COLLABORATION: Both users interact with thread ===
    console.log('\n🤝 COLLABORATION: Both users adding steps...');

    // Host adds a step with refs
    console.log('📝 Host adding step with refs...');
    await hostThread
      .step('process_payment')
      .context({ amount: '$49.99' })
      .addRefs({ stripe_payment_id: 'pi_12345' })
      .success('Host processed data', { result: 'Host processed data' });
    console.log('✅ Host step completed');

    // Guest adds a step (if permissions allow)
    if (guestThread.permissions.includes('write')) {
      console.log('📝 Guest adding step...');
      await guestThread
        .step('review_data')
        .addContext({ type: 'guest_review' })
        .success('Guest reviewed data', { result: 'Guest reviewed data' });
      console.log('✅ Guest step completed');

      // Example of error handling
      console.log('📝 Guest testing error handling...');
      await guestThread
        .step('test_error')
        .addContext({ type: 'error_test' })
        .error('Test error occurred', { errorCode: 'TEST_ERROR', details: 'This is a test error' });
      console.log('✅ Error step completed');
    } else {
      console.log('📝 Guest has read-only permissions, cannot add steps');
    }

    // === CLEANUP ===
    console.log('\n🧹 Cleaning up...');
    await hostThread.close();
    await guestThread.close();
    console.log('✅ Connections closed');

    console.log('\n🎉 Invitation system example completed successfully!');

  } catch (error) {
    console.error('❌ Error in invitation example:', error.message);
    console.error('Stack:', error.stack);
  }
}

// === ERROR HANDLING EXAMPLES ===
async function errorHandlingExamples() {
  console.log('\n🛠️  Error Handling Examples\n');

  // Reuse connection for multiple tests
  const testThread = await Threadify.connect('test-api-key');

  // Example 1: Invalid role
  try {
    await testThread.start('payment_flow', 'invalid_role', { refs: { test: true } });
    await testThread.inviteParty({ role: 'invalid_role' });
  } catch (error) {
    console.log('✅ Caught invalid role error:', error.message);
  }

  // Example 2: Missing required parameters
  try {
    await testThread.join('');
  } catch (error) {
    console.log('✅ Caught missing token error:', error.message);
  }

  // Example 3: Invalid token
  try {
    await testThread.join('invalid-token');
  } catch (error) {
    console.log('✅ Caught invalid token error:', error.message);
  }

  await testThread.close();
}

// === PERMISSION TESTING ===
async function permissionTesting() {
  console.log('\n🔐 Permission Testing\n');

  try {
    // Create thread and invite with read-only permissions (without contract)
    const hostThread = await Threadify.connect('test-api-key');
    await hostThread.start('payment_flow', 'merchant', {
      refs: {
        customer_id: '12345',
        order_id: 'order-67890'
      }
    });
    
    const readOnlyToken = await hostThread.inviteParty({
      role: 'auditor',
      permissions: 'read'
    });

    // Join with read-only permissions
    const readOnlyThread = await Threadify.connect('test-api-key', 'auditor-service');
    await readOnlyThread.join(readOnlyToken);
    console.log(`👤 Auditor joined with permissions: ${readOnlyThread.permissions}`);

    // Note: In a real implementation, step creation would be checked against permissions
    // This is mentioned in the requirements but not implemented yet
    console.log('📝 Permission checking for step creation would happen here');

    await hostThread.close();
    await readOnlyThread.close();
    
  } catch (error) {
    console.error('❌ Error in permission testing:', error.message);
  }
}

// Run examples
if (import.meta.url === `file://${process.argv[1]}`) {
  invitationExample()
    .then(() => errorHandlingExamples())
    .then(() => permissionTesting())
    .catch(console.error);
}

export { invitationExample, errorHandlingExamples, permissionTesting };
