'use strict';
// Held-out fixture app: different shape from demo-app on purpose.
const fs = require('node:fs');
const key = process.env.API_KEY || '';
const storeDir = '/data/files';
function save(name, data) {
  fs.mkdirSync(storeDir, { recursive: true });
  fs.writeFileSync(storeDir + '/' + name, data);
  return key.length > 0;
}
module.exports = { save };
