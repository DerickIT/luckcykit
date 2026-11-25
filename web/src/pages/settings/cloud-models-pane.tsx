import { Alert, App, Button, Card, Form, Input, List, Spin, Tag } from "antd";
import { LogIn, LogOut, QrCode, RefreshCw } from "lucide-react";
import { useEffect, useState } from "react";

import { PageHeader } from "@/components/layout/workspace-page";
import { useCloudSessionStore } from "@/stores/use-cloud-session-store";

const CAPABILITY_LABELS: Record<string, string> = { text: "文本", image: "图片", video: "视频", audio: "音频" };
const LEDGER_KIND_LABELS: Record<string, string> = {
    sandbox_purchase: "沙箱充值",
    sandbox_payment: "沙箱充值",
    hosted_model_charge: "托管模型扣费",
    hosted_model_reservation: "额度预留",
    hosted_model_release: "额度释放",
    refund: "退款",
};

export function CloudModelsPane() {
    const { message } = App.useApp();
    const baseURL = useCloudSessionStore((state) => state.baseURL);
    const email = useCloudSessionStore((state) => state.email);
    const token = useCloudSessionStore((state) => state.token);
    const account = useCloudSessionStore((state) => state.account);
    const models = useCloudSessionStore((state) => state.models);
    const ledger = useCloudSessionStore((state) => state.ledger);
    const loading = useCloudSessionStore((state) => state.loading);
    const error = useCloudSessionStore((state) => state.error);
    const setBaseURL = useCloudSessionStore((state) => state.setBaseURL);
    const setEmail = useCloudSessionStore((state) => state.setEmail);
    const requestCode = useCloudSessionStore((state) => state.requestCode);
    const verifyCode = useCloudSessionStore((state) => state.verifyCode);
    const loginWithWeChat = useCloudSessionStore((state) => state.loginWithWeChat);
    const refresh = useCloudSessionStore((state) => state.refresh);
    const logout = useCloudSessionStore((state) => state.logout);
    const [codeSent, setCodeSent] = useState(false);
    const [code, setCode] = useState("");
    const [cooldown, setCooldown] = useState(0);

    useEffect(() => {
        let cancelled = false;
        void (async () => {
            await useCloudSessionStore.getState().hydrate();
            // hydrate only reads the stored session; network loading stays here so
            // a missing cloud endpoint never blocks workspace startup.
            if (!cancelled && useCloudSessionStore.getState().token) await useCloudSessionStore.getState().refresh();
        })();
        return () => {
            cancelled = true;
        };
    }, []);

    // Editing the address or email invalidates an already-requested code.
    useEffect(() => {
        setCodeSent(false);
        setCode("");
        setCooldown(0);
    }, [email]);

    useEffect(() => {
        if (cooldown <= 0) return;
        const timer = window.setTimeout(() => setCooldown((value) => value - 1), 1_000);
        return () => window.clearTimeout(timer);
    }, [cooldown]);

    const submitCode = async () => {
        try {
            await requestCode();
            setCodeSent(true);
            setCooldown(60);
            message.success("验证码已发送，请查收邮箱");
        } catch {
            // Error is rendered by the pane.
        }
    };

    const submitLogin = async () => {
        try {
            await verifyCode(code);
            setCode("");
            setCodeSent(false);
            setCooldown(0);
            message.success("云端账号已登录");
        } catch {
            // Error is rendered by the pane.
        }
    };

    const submitWeChatLogin = async () => {
        try {
            await loginWithWeChat();
            message.success("微信云端账号已登录");
        } catch {
            // Error is rendered by the pane.
        }
    };

    const points = account?.points;
    const available = account?.availablePoints;
    const reserved = account?.reservedPoints;

    return (
        <div className="settings-pane">
            <PageHeader title="云端模型" description="登录独立 LuckyTV 云服务，使用托管文本模型。云端账号、点数和本地模型渠道相互独立。" />
            {error ? <Alert className="mb-4" type="error" showIcon message={error} /> : null}
            {!token ? (
                <Card className="settings-section" title="云端登录" extra={<LogIn className="size-4" />}>
                    <Form layout="vertical" requiredMark={false} onFinish={() => void (codeSent ? submitLogin() : submitCode())}>
                        <Form.Item label="云服务 API 地址" htmlFor="cloud-api-base-url" extra="桌面端会记住地址；浏览器不会保存云端访问令牌。">
                            <Input id="cloud-api-base-url" value={baseURL} onChange={(event) => setBaseURL(event.target.value)} placeholder="https://cloud.example.com/api/v1" />
                        </Form.Item>
                        <Button type="primary" icon={<QrCode className="size-4" />} loading={loading} onClick={() => void submitWeChatLogin()}>
                            微信扫码登录
                        </Button>
                        <div className="mt-4 border-t border-border pt-4">
                            <div className="mb-2 text-sm text-foreground/60">本地沙箱备用登录</div>
                            <Form.Item label="邮箱" htmlFor="cloud-account-email">
                                <Input id="cloud-account-email" type="email" value={email} onChange={(event) => setEmail(event.target.value)} placeholder="name@example.com" autoComplete="email" />
                            </Form.Item>
                            {codeSent ? (
                                <Form.Item label="验证码" htmlFor="cloud-account-code">
                                    <Input id="cloud-account-code" value={code} onChange={(event) => setCode(event.target.value.replace(/\D/gu, "").slice(0, 6))} inputMode="numeric" maxLength={6} placeholder="输入邮箱收到的 6 位验证码" autoComplete="one-time-code" />
                                </Form.Item>
                            ) : null}
                            <Button htmlType="submit" loading={loading} disabled={!email.trim() || (codeSent && code.length !== 6)}>
                                {codeSent ? "登录沙箱" : "发送沙箱验证码"}
                            </Button>
                            {codeSent ? <Button className="ml-2" disabled={cooldown > 0} onClick={() => void submitCode()}>{cooldown > 0 ? `重新发送（${cooldown}s）` : "重新发送"}</Button> : null}
                        </div>
                    </Form>
                </Card>
            ) : (
                <>
                    <Card className="settings-section mb-4" title="云端账户" extra={<Button icon={<LogOut className="size-4" />} onClick={() => void logout()}>退出登录</Button>}>
                        <div className="flex flex-col gap-1">
                            <span>{email}</span>
                            <span className="text-sm text-foreground/60">
                                点数：{points ?? "—"}
                                {available != null ? ` · 可用 ${available}` : ""}
                                {reserved != null && reserved > 0 ? ` · 预留 ${reserved}` : ""}
                            </span>
                            <span className="text-sm text-foreground/60">云服务地址：{baseURL}</span>
                        </div>
                    </Card>
                    <section className="settings-section">
                        <div className="settings-pane-header flex items-center justify-between">
                            <div><h2>可用云模型</h2><p>每次生成时选择云端来源；首期仅开放文本托管任务。</p></div>
                            <Button icon={<RefreshCw className="size-4" />} loading={loading} onClick={() => void refresh()}>刷新</Button>
                        </div>
                        <List
                            loading={loading && !models.length}
                            dataSource={models}
                            locale={{ emptyText: "暂无可用云模型" }}
                            renderItem={(model) => (
                                <List.Item>
                                    <List.Item.Meta
                                        title={
                                            <span className="flex flex-wrap items-center gap-2">
                                                {model.displayName}
                                                {(model.capabilities.length ? model.capabilities : ["text"]).map((capability) => (
                                                    <Tag key={capability}>{CAPABILITY_LABELS[capability] ?? capability}</Tag>
                                                ))}
                                            </span>
                                        }
                                        description={`模型 ${model.id} · 输入上限 ${model.maxInputTokens.toLocaleString()} · 输出上限 ${model.maxOutputTokens.toLocaleString()} · 价格版本 ${model.priceVersion}`}
                                    />
                                    <span className="text-sm text-foreground/60">输入 {model.inputPointsPerMillion} 点/百万 token · 输出 {model.outputPointsPerMillion} 点/百万 token</span>
                                </List.Item>
                            )}
                        />
                    </section>
                    <section className="settings-section mt-4">
                        <div className="settings-pane-header"><h2>最近点数账本</h2><p>点数和扣费以云服务账本为准。</p></div>
                        <List
                            size="small"
                            dataSource={ledger.slice(0, 10)}
                            locale={{ emptyText: "暂无账本记录" }}
                            renderItem={(entry) => (
                                <List.Item>
                                    <span className="flex min-w-0 flex-col">
                                        <span>{LEDGER_KIND_LABELS[entry.kind] ?? entry.kind}</span>
                                        <span className="text-xs text-foreground/60">{Number.isNaN(Date.parse(entry.createdAt)) ? "" : new Date(entry.createdAt).toLocaleString("zh-CN")}</span>
                                    </span>
                                    <span className={entry.points < 0 ? "text-red-500" : "text-green-600"}>{entry.points > 0 ? "+" : ""}{entry.points}</span>
                                </List.Item>
                            )}
                        />
                    </section>
                </>
            )}
            {loading && token ? <Spin className="mt-4" size="small" /> : null}
        </div>
    );
}