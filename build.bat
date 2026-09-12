@echo off
echo ========================================
echo Building OCI Panel (Full Stack)
echo ========================================

echo.
echo [1/2] Building Frontend (Lumina Style Studio)...
cd frontend
call npm install
if errorlevel 1 (
    echo Frontend dependencies installation failed!
    cd ..
    pause
    exit /b 1
)

call npm run build
if errorlevel 1 (
    echo Frontend build failed!
    cd ..
    pause
    exit /b 1
)
echo Frontend build completed: frontend/dist/
cd ..

echo.
echo [2/2] Building Backend (Go)...
go mod tidy
go build -ldflags "-s -w" -o oci-panel.exe main.go
if errorlevel 1 (
    echo Backend build failed!
    pause
    exit /b 1
)

echo.
echo ========================================
echo Build completed successfully!
echo - Backend: oci-panel.exe
echo - Frontend: frontend/dist/
echo.
echo Run 'oci-panel.exe' to start the server
echo Then open http://localhost:8818 in browser
echo ========================================
pause
