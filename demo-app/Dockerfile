FROM node:20-alpine
WORKDIR /app
COPY package.json package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY tsconfig.json ./
COPY src ./src
COPY migrations ./migrations
RUN npm run build
RUN mkdir -p /data/uploads && chown -R node:node /data
EXPOSE 3000
USER node
CMD ["node", "dist/index.js"]
