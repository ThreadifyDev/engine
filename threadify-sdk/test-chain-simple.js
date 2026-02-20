import { Threadify } from './src/index.js';

async function main() {
    console.log('Connecting...');
    const conn = await Threadify.connect('api-key-123', 'test', { url: 'ws://localhost:8081/threads' });
    
    console.log('Creating root thread...');
    const root = await conn.start(null, 'svc');
    await root.step('s1').success();
    
    console.log('Creating child thread...');
    const child = await conn.start(null, 'svc');
    await child.linkThread(root.threadId, 'parent');
    await child.step('s2').success();
    
    console.log('\nROOT:', root.threadId);
    console.log('CHILD:', child.threadId);
    
    console.log('\nWaiting for persistence...');
    await new Promise(r => setTimeout(r, 2000));
    
    console.log('\nQuerying threadChain...');
    try {
        const chain = await conn.getThreadChain(root.threadId, 3);
        console.log('SUCCESS! Chain length:', chain.length);
        console.log('Thread IDs:', chain.map(t => t.id));
    } catch (err) {
        console.error('ERROR:', err.message);
    }
    
    conn.ws.close();
    process.exit(0);
}

main().catch(e => { console.error('FATAL:', e); process.exit(1); });
