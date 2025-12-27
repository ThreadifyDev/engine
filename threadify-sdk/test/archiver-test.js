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
    console.log('⏳ Waiting 10 seconds for archiver to process...\n');
    
    // Wait for archiver to process
    await new Promise(resolve => setTimeout(resolve, 10000));
    
    console.log('✅ Test complete! Check archiver logs for processing details.\n');
    
    // Close connection
    await connection.close();
    
  } catch (error) {
    console.error('❌ Error:', error.message);
    process.exit(1);
  }
}

testArchiver();
