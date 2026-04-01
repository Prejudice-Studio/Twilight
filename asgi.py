import os
import platform as _platform
_platform._wmi = None  # 规避 Windows WMI 服务无响应导致 SQLAlchemy 导入卡死

from src.api import create_app
from asgiref.wsgi import WsgiToAsgi

# Create the Flask application
flask_app = create_app()

# Wrap it for ASGI servers (Uvicorn, Hypercorn)
app = WsgiToAsgi(flask_app)

if __name__ == "__main__":
    import uvicorn
    # Allow running this file directly for development
    uvicorn.run("asgi:app", host="0.0.0.0", port=5000, reload=True)
