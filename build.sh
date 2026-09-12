#!/bin/bash
set -e

echo "========================================"
echo "Building OCI Panel (Full Stack)"
echo "========================================"

echo ""
echo "[1/2] Building Frontend (Lumina Style Studio)..."
cd frontend
npm install
npm run build
echo "Frontend build completed: frontend/dist/"
cd ..

echo ""
echo "[2/2] Building Backend (Go)..."
go mod download
go build -ldflags "-s -w" -o oci-panel main.go

echo ""
echo "========================================"
echo "Build completed successfully!"
echo "- Backend: oci-panel"
echo "- Frontend: frontend/dist/"
echo ""
echo "Run './oci-panel' to start the server"
echo "Then open http://localhost:8818 in browser"
echo "========================================"
