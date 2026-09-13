// Runtime auth override for competition demo.
// This script intentionally overrides legacy login behavior in chat-common.js.

(function () {
    if (typeof window === 'undefined') {
        return;
    }

    window.quickLogin = async function quickLogin(userName, userId) {
        if (typeof clearAuthError === 'function') {
            clearAuthError();
        }

        console.log(`[auth-override] login start: ${userName} (${userId})`);

        if (window.CONFIG && window.CONFIG.OFFLINE_MODE) {
            if (typeof loginWithExplicitOfflineMode === 'function') {
                loginWithExplicitOfflineMode(userName, userId);
            }
            return;
        }

        const password = typeof getDemoPassword === 'function' ? getDemoPassword() : '';
        if (!password) {
            if (typeof renderSecurityNotice === 'function') {
                renderSecurityNotice();
            }
            if (typeof showAuthError === 'function') {
                showAuthError('Demo password is required before login.');
            }
            const passwordInput = document.getElementById('demoPasswordInput');
            if (passwordInput) {
                passwordInput.focus();
            }
            return;
        }

        const loginOverlay = document.getElementById('loginOverlay');
        const loginStatus = document.getElementById('loginStatusMessage');
        if (loginStatus) {
            loginStatus.style.display = 'none';
        }

        try {
            if (loginOverlay) {
                loginOverlay.style.pointerEvents = 'none';
                loginOverlay.style.opacity = '0.96';
            }

            const response = await fetch(`${window.CONFIG ? window.CONFIG.API_BASE_URL : 'http://localhost:8088'}/api/auth/login`, {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({
                    user_id: userId,
                    password: password,
                    workspace_id: window.CONFIG ? window.CONFIG.DEFAULT_WORKSPACE : 'default'
                })
            });

            if (!response.ok) {
                throw new Error(`HTTP ${response.status}`);
            }

            const data = await response.json();
            if (!data.success || !data.data || !data.data.token) {
                throw new Error('Invalid login response');
            }

            JWT_TOKEN = data.data.token;
            USER_ID = userId;
            USER_NAME = userName;

            if (typeof persistAuth === 'function') {
                persistAuth(JWT_TOKEN, USER_ID, USER_NAME);
            }

            const userAvatar = document.getElementById('userAvatar');
            if (userAvatar) {
                userAvatar.textContent = userName;
            }

            if (loginOverlay) {
                loginOverlay.style.display = 'none';
                loginOverlay.style.pointerEvents = '';
                loginOverlay.style.opacity = '';
            }

            if (typeof initializeAfterLogin === 'function') {
                initializeAfterLogin();
            }
        } catch (error) {
            console.error('[auth-override] login failed:', error);
            if (loginOverlay) {
                loginOverlay.style.pointerEvents = '';
                loginOverlay.style.opacity = '';
            }
            if (typeof renderSecurityNotice === 'function') {
                renderSecurityNotice();
            }
            if (typeof showAuthError === 'function') {
                const apiBase = window.CONFIG ? window.CONFIG.API_BASE_URL : 'http://localhost:8088';
                const detail = error && error.message === 'Failed to fetch'
                    ? `无法连接服务 ${apiBase}。请确认 Docker/服务已启动，或用 ?api=http://127.0.0.1:8088 指定地址。`
                    : `服务返回登录失败（${error.message}）。请检查演示密码和服务日志。`;
                showAuthError(`${detail} 如需纯本地展示，请在 web/js/config.js 中明确开启 OFFLINE_MODE。`);
            }
        }
    };
})();
