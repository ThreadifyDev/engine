import { Threadify } from '../src/index.js';

async function testArchiver() {
  console.log('🚀 Testing Archiver Integration...\n');

  try {
    // Connect with a valid API key
    console.log('📡 Connecting to Threadify...');
    const connection = await Threadify.connect('api-key-123', 'archiver-test-service');
    console.log('✅ Connected successfully!\n');

    // Start a thread
    console.log('🏁 Starting thread...');
    const thread = await connection.start();
    console.log(`✅ Thread started: ${thread.threadId}\n`);

    // Record multiple step events with delay after 3rd event
    console.log('📝 Recording step events with timing metrics...\n');
    
    for (let i = 1; i <= 5; i++) {
      const startTime = Date.now();
      console.log(`   Step ${i}/5...`);
      
      await thread
        .step(`test-step-${i}`)
        .addContext({ 
          stepNumber: i,
          testData: `data-${i}`,
          timestamp: new Date().toISOString(),
          clientSentAt: Date.now()
        })
        .stop('success', `Step ${i} completed`);
      
      const endTime = Date.now();
      const duration = endTime - startTime;
      console.log(`   ⏱️  Step ${i} completed in ${duration}ms (API write time)\n`);
      
      // Add 6 second delay after 3rd event
      if (i === 3) {
        console.log('⏸️  Waiting 6 seconds after 3rd event (should trigger partial flush)...\n');
        await new Promise(resolve => setTimeout(resolve, 6000));
      }
    }
    
    console.log('✅ All 5 steps recorded!\n');
    
    // Test invitation and joining
    console.log('🎫 Creating invitation...');
    // inviteParty is now on the thread instance
    const inviteToken = await thread.inviteParty({
      role: 'external_partner',
      permissions: 'read,write',
      expiresIn: '24h'
    });
    console.log(`✅ Invitation created: ${inviteToken.substring(0, 20)}...\n`);
    
    // Simulate another user joining with the token
    console.log('👥 Simulating user joining thread...');
    const connection2 = await Threadify.connect('api-key-456', 'partner-service');
    console.log('✅ Second user connected!\n');
    
    const joinedThread = await connection2.join(inviteToken);
    console.log(`✅ User joined thread: ${joinedThread.threadId}`);
    console.log(`   Role: ${joinedThread.role}`);
    console.log(`   Can now use joinedThread.step() to interact with the thread\n`);
    
    console.log('⏳ Waiting 10 seconds for archiver to process all streams...\n');
    
    // Wait for archiver to process
    await new Promise(resolve => setTimeout(resolve, 10000));
    
    console.log('✅ Test complete! Check archiver logs for processing details.\n');
    
    // Close connections
    await connection2.close();
    await connection.close();
    
  } catch (error) {
    console.error('❌ Error:', error.message);
    process.exit(1);
  }
}

testArchiver();
