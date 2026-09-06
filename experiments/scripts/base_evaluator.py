#!/usr/bin/env python
# -*- coding: utf-8 -*-
"""
基础评估器 - 统一错误处理和API响应验证
"""

import json
import os
import hashlib
import requests
from typing import Dict, Any, Optional, Tuple
from pathlib import Path
from dataclasses import dataclass
from datetime import datetime


@dataclass
class APIResponse:
    """API响应封装"""
    success: bool
    http_status: int
    data: Optional[Any]
    error_type: Optional[str]  # timeout, connection_error, http_error, parse_error, empty_response
    error_message: Optional[str]
    latency_ms: float

    def is_valid_business_response(self) -> bool:
        """判断是否为有效的业务响应（可计入指标）"""
        return (
            self.success and
            200 <= self.http_status < 300 and
            self.data is not None and
            len(self.data) > 0 and
            self.error_type is None
        )


class BaseEvaluator:
    """基础评估器"""

    def __init__(self, base_url: str = "http://localhost:8088"):
        self.base_url = base_url
        self.default_timeout = 120  # 增加到120秒以支持检索操作
        self._jwt_token = None

    def call_api(
        self,
        method: str,
        endpoint: str,
        payload: Optional[Dict] = None,
        headers: Optional[Dict] = None,
        timeout: Optional[int] = None
    ) -> APIResponse:
        """
        统一API调用方法，包含完整错误处理

        Returns:
            APIResponse: 包含http_status、error_type、error_message的响应对象
        """
        url = f"{self.base_url}{endpoint}"
        timeout = timeout or self.default_timeout
        headers = headers or {}

        start_time = datetime.now()

        try:
            if method.upper() == "POST":
                response = requests.post(url, json=payload, headers=headers, timeout=timeout)
            elif method.upper() == "GET":
                response = requests.get(url, params=payload, headers=headers, timeout=timeout)
            elif method.upper() == "DELETE":
                response = requests.delete(url, json=payload, headers=headers, timeout=timeout)
            else:
                raise ValueError(f"不支持的HTTP方法: {method}")

            latency_ms = (datetime.now() - start_time).total_seconds() * 1000

            # 记录HTTP状态
            http_status = response.status_code

            # 4xx/5xx错误
            if http_status >= 400:
                error_type = "http_error"
                error_message = f"HTTP {http_status}: {response.text[:200]}"
                return APIResponse(
                    success=False,
                    http_status=http_status,
                    data=None,
                    error_type=error_type,
                    error_message=error_message,
                    latency_ms=latency_ms
                )

            # 尝试解析JSON
            try:
                data = response.json()
            except json.JSONDecodeError as e:
                return APIResponse(
                    success=False,
                    http_status=http_status,
                    data=None,
                    error_type="parse_error",
                    error_message=f"JSON解析失败: {str(e)}",
                    latency_ms=latency_ms
                )

            # 检查空响应
            if data is None or (isinstance(data, dict) and len(data) == 0):
                return APIResponse(
                    success=False,
                    http_status=http_status,
                    data=data,
                    error_type="empty_response",
                    error_message="API返回空响应",
                    latency_ms=latency_ms
                )

            # 成功
            return APIResponse(
                success=True,
                http_status=http_status,
                data=data,
                error_type=None,
                error_message=None,
                latency_ms=latency_ms
            )

        except requests.exceptions.Timeout:
            latency_ms = (datetime.now() - start_time).total_seconds() * 1000
            return APIResponse(
                success=False,
                http_status=0,
                data=None,
                error_type="timeout",
                error_message=f"请求超时 (>{timeout}s)",
                latency_ms=latency_ms
            )

        except requests.exceptions.ConnectionError as e:
            latency_ms = (datetime.now() - start_time).total_seconds() * 1000
            return APIResponse(
                success=False,
                http_status=0,
                data=None,
                error_type="connection_error",
                error_message=f"连接失败: {str(e)}",
                latency_ms=latency_ms
            )

        except Exception as e:
            latency_ms = (datetime.now() - start_time).total_seconds() * 1000
            return APIResponse(
                success=False,
                http_status=0,
                data=None,
                error_type="unknown_error",
                error_message=f"未知错误: {str(e)}",
                latency_ms=latency_ms
            )

    def authenticate_from_env(self) -> APIResponse:
        """Load an evaluation token or login with environment credentials."""
        token = os.getenv("EVAL_AUTH_TOKEN", "").strip()
        if token:
            self._jwt_token = token
            return APIResponse(True, 200, {"token_source": "environment"}, None, None, 0.0)

        user_id = os.getenv("EVAL_USER_ID", "").strip()
        password = os.getenv("EVAL_PASSWORD", "")
        if not user_id or not password:
            return APIResponse(
                False, 0, None, "auth_not_configured",
                "Set EVAL_AUTH_TOKEN or EVAL_USER_ID and EVAL_PASSWORD for protected API tests.", 0.0,
            )

        response = self.call_api("POST", "/api/auth/login", {
            "user_id": user_id,
            "password": password,
            "workspace_id": os.getenv("EVAL_WORKSPACE_ID", "default"),
        })
        if not response.is_valid_business_response() or not isinstance(response.data, dict):
            return response

        login_data = response.data.get("data", response.data)
        token = login_data.get("token") if isinstance(login_data, dict) else None
        if not token:
            return APIResponse(
                False, response.http_status, response.data, "auth_response_invalid",
                "Login succeeded but did not return a token.", response.latency_ms,
            )

        self._jwt_token = token
        return response

    def get_auth_headers(self) -> Dict[str, str]:
        """获取认证header"""
        if self._jwt_token:
            return {"Authorization": f"Bearer {self._jwt_token}"}
        return {}

    def check_service_health(self) -> Tuple[bool, str]:
        """检查服务健康状态"""
        response = self.call_api("GET", "/health")
        if response.is_valid_business_response():
            return True, "服务正常"
        else:
            return False, f"服务异常: {response.error_message}"

    @staticmethod
    def calculate_config_hash(config_path: str) -> str:
        """
        计算配置文件的SHA-256 hash

        Args:
            config_path: 配置文件路径

        Returns:
            str: 64字符的十六进制hash字符串
        """
        try:
            with open(config_path, 'rb') as f:
                file_hash = hashlib.sha256(f.read()).hexdigest()
            return file_hash
        except Exception as e:
            return f"error_calculating_hash_{str(e)[:20]}"

    @staticmethod
    def get_config_path(config_name: str) -> Optional[str]:
        """
        获取配置文件的完整路径

        Args:
            config_name: 配置名称（不带.yaml扩展名）

        Returns:
            str: 配置文件完整路径，如果不存在返回None
        """
        # 从脚本目录找到配置目录
        script_dir = Path(__file__).parent
        configs_dir = script_dir.parent / "configs"
        config_path = configs_dir / f"{config_name}.yaml"

        if config_path.exists():
            return str(config_path)
        return None
