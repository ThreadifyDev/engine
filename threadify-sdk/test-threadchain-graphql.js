import { Threadify } from './src/index.js';

async function test() {
    const conn = await Threadify.connect('api-key-123', 'test-graphql', { url: 'ws://localhost:8081/threads' });
    
    // Create root thread
    const root = await conn.start(null, 'root-service');
    await root.step('step1').addContext({ test: 'root' }).success();
    console.log('Root thread:', root.threadId);
    
    // Create child thread
    const child = await conn.start(null, 'child-service');
    await child.linkThread(root.threadId, 'parent');
    await child.step('step2').addContext({ test: 'child' }).success();
    console.log('Child thread:', child.threadId);
    
    // Create grandchild thread
    const grandchild = await conn.start(null, 'grandchild-service');
    await grandchild.linkThread(child.threadId, 'parent');
    await grandchild.step('step3').addContext({ test: 'grandchild' }).success();
    console.log('Grandchild thread:', grandchild.threadId);
    
    // Wait for persistence
    await new Promise(resolve => setTimeout(resolve, 2000));
    
    // Test GraphQL query
    console.log('\nTesting threadChain query...');
    const chain = await conn.getThreadChain(root.threadId, 3);
    console.log('Chain length:', chain.length);
    console.log('Thread IDs:', chain.map(t => t.id));
    
    conn.ws.close();
    process.exit(0);
}

test().catch(err => {
    console.error('Error:', err);
    process.exit(1);
});
