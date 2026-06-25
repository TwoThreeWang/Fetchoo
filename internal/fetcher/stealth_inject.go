package fetcher

import (
	"strings"

	"github.com/go-rod/rod"
	"web_fetcher/internal/fingerprint"
)

const stealthScriptTemplate = `
(function() {
    // 1. 伪装 webdriver
    Object.defineProperty(navigator, 'webdriver', {
        get: () => false,
        set: () => {},
        configurable: true
    });

    // 2. 伪装 window.chrome
    if (!window.chrome) {
        window.chrome = {
            runtime: {}
        };
    }

    // 3. 伪装 plugins（使用真实浏览器的插件列表）
    if (navigator.plugins.length === 0) {
        Object.defineProperty(navigator, 'plugins', {
            get: () => [
                { name: 'Chrome PDF Plugin', description: 'Portable Document Format' },
                { name: 'Chrome PDF Viewer', description: '' },
                { name: 'Native Client Executable', description: '' },
            ],
            configurable: true
        });
    }

    // 4. 修复 Function.prototype.toString 检测
    const originalFunctionToString = Function.prototype.toString;
    Function.prototype.toString = function() {
        if (this === fetch || this === XMLHttpRequest) {
            return 'function ' + this.name + '() { [native code] }';
        }
        return originalFunctionToString.call(this);
    };

    // 5. 随机化 Canvas 指纹 (极轻微的噪声，改变 toDataURL/getImageData)
    const originalToDataURL = HTMLCanvasElement.prototype.toDataURL;
    HTMLCanvasElement.prototype.toDataURL = function(type, ...args) {
        const ctx = this.getContext('2d');
        if (ctx && this.width > 0 && this.height > 0) {
            ctx.fillStyle = 'rgba(' + Math.random() * 255 + ',' + Math.random() * 255 + ',' + Math.random() * 255 + ',0.01)';
            ctx.fillRect(0, 0, 1, 1);
        }
        return originalToDataURL.call(this, type, ...args);
    };

    const originalGetImageData = CanvasRenderingContext2D.prototype.getImageData;
    CanvasRenderingContext2D.prototype.getImageData = function(x, y, w, h) {
        const imageData = originalGetImageData.call(this, x, y, w, h);
        if (imageData && imageData.data && imageData.data.length > 0) {
            // 仅仅翻转第一个像素的一个通道的极小值，足够改变哈希
            const idx = Math.floor(Math.random() * 4);
            imageData.data[idx] = imageData.data[idx] ^ 1;
        }
        return imageData;
    };

    // 6. 修复 WebGL 渲染器信息 (对应 profile)
    const getParameterProxyHandler = {
        apply: function(target, ctx, args) {
            const param = args[0];
            // 37445 = UNMASKED_VENDOR_WEBGL
            if (param === 37445) {
                return 'Google Inc. (Apple)';
            }
            // 37446 = UNMASKED_RENDERER_WEBGL
            if (param === 37446) {
                return '%GPU_RENDERER%';
            }
            return Reflect.apply(target, ctx, args);
        }
    };
    if (WebGLRenderingContext && WebGLRenderingContext.prototype.getParameter) {
        WebGLRenderingContext.prototype.getParameter = new Proxy(
            WebGLRenderingContext.prototype.getParameter,
            getParameterProxyHandler
        );
    }
    if (WebGL2RenderingContext && WebGL2RenderingContext.prototype.getParameter) {
        WebGL2RenderingContext.prototype.getParameter = new Proxy(
            WebGL2RenderingContext.prototype.getParameter,
            getParameterProxyHandler
        );
    }
})();
`

func injectStealthScript(page *rod.Page, profile fingerprint.BrowserProfile) error {
	script := strings.Replace(stealthScriptTemplate, "%GPU_RENDERER%", profile.GPURenderer, 1)

	// 使用 EvalOnNewDocument 在每个新文档加载前执行 JS
	_, err := page.EvalOnNewDocument(script)
	return err
}
