-- Context-Keeper TimescaleDB 初始化脚本
-- 用于时间线存储和时序数据管理

-- 启用 TimescaleDB 扩展
CREATE EXTENSION IF NOT EXISTS timescaledb CASCADE;

-- 创建时间线事件表
CREATE TABLE IF NOT EXISTS timeline_events (
    id BIGSERIAL,
    event_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    session_id VARCHAR(255) NOT NULL,
    user_id VARCHAR(255) NOT NULL,
    workspace_id VARCHAR(255),
    event_type VARCHAR(50) NOT NULL,
    event_category VARCHAR(50),
    content TEXT,
    metadata JSONB,
    embedding_id VARCHAR(255),
    importance_score FLOAT DEFAULT 0.5,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (event_time, id)
);

-- 将表转换为 TimescaleDB 超表（hypertable）
SELECT create_hypertable('timeline_events', 'event_time',
    chunk_time_interval => INTERVAL '1 day',
    if_not_exists => TRUE
);

-- 创建索引以优化查询性能
CREATE INDEX IF NOT EXISTS idx_timeline_session_id ON timeline_events (session_id, event_time DESC);
CREATE INDEX IF NOT EXISTS idx_timeline_user_id ON timeline_events (user_id, event_time DESC);
CREATE INDEX IF NOT EXISTS idx_timeline_workspace_id ON timeline_events (workspace_id, event_time DESC);
CREATE INDEX IF NOT EXISTS idx_timeline_event_type ON timeline_events (event_type, event_time DESC);
CREATE INDEX IF NOT EXISTS idx_timeline_importance ON timeline_events (importance_score DESC, event_time DESC);
CREATE INDEX IF NOT EXISTS idx_timeline_metadata ON timeline_events USING GIN (metadata);

-- 创建对话消息表
CREATE TABLE IF NOT EXISTS conversation_messages (
    id BIGSERIAL,
    message_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    session_id VARCHAR(255) NOT NULL,
    user_id VARCHAR(255) NOT NULL,
    role VARCHAR(20) NOT NULL,
    content TEXT NOT NULL,
    token_count INTEGER,
    metadata JSONB,
    parent_message_id BIGINT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (message_time, id)
);

-- 转换为超表
SELECT create_hypertable('conversation_messages', 'message_time',
    chunk_time_interval => INTERVAL '1 day',
    if_not_exists => TRUE
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_conv_session_id ON conversation_messages (session_id, message_time DESC);
CREATE INDEX IF NOT EXISTS idx_conv_user_id ON conversation_messages (user_id, message_time DESC);
CREATE INDEX IF NOT EXISTS idx_conv_role ON conversation_messages (role, message_time DESC);

-- 创建上下文快照表
CREATE TABLE IF NOT EXISTS context_snapshots (
    id BIGSERIAL,
    snapshot_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    session_id VARCHAR(255) NOT NULL,
    user_id VARCHAR(255) NOT NULL,
    workspace_id VARCHAR(255),
    context_type VARCHAR(50) NOT NULL,
    context_data JSONB NOT NULL,
    version INTEGER DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (snapshot_time, id)
);

-- 转换为超表
SELECT create_hypertable('context_snapshots', 'snapshot_time',
    chunk_time_interval => INTERVAL '7 days',
    if_not_exists => TRUE
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_snapshot_session_id ON context_snapshots (session_id, snapshot_time DESC);
CREATE INDEX IF NOT EXISTS idx_snapshot_user_id ON context_snapshots (user_id, snapshot_time DESC);
CREATE INDEX IF NOT EXISTS idx_snapshot_type ON context_snapshots (context_type, snapshot_time DESC);

-- 创建会话统计表
CREATE TABLE IF NOT EXISTS session_statistics (
    id BIGSERIAL,
    stat_time TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    session_id VARCHAR(255) NOT NULL,
    user_id VARCHAR(255) NOT NULL,
    message_count INTEGER DEFAULT 0,
    token_count INTEGER DEFAULT 0,
    duration_seconds INTEGER DEFAULT 0,
    active_duration_seconds INTEGER DEFAULT 0,
    metrics JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (stat_time, id)
);

-- 转换为超表
SELECT create_hypertable('session_statistics', 'stat_time',
    chunk_time_interval => INTERVAL '1 day',
    if_not_exists => TRUE
);

-- 创建索引
CREATE INDEX IF NOT EXISTS idx_stats_session_id ON session_statistics (session_id, stat_time DESC);
CREATE INDEX IF NOT EXISTS idx_stats_user_id ON session_statistics (user_id, stat_time DESC);

-- 创建数据保留策略（自动删除90天前的数据）
SELECT add_retention_policy('timeline_events', INTERVAL '90 days', if_not_exists => TRUE);
SELECT add_retention_policy('conversation_messages', INTERVAL '90 days', if_not_exists => TRUE);
SELECT add_retention_policy('context_snapshots', INTERVAL '180 days', if_not_exists => TRUE);
SELECT add_retention_policy('session_statistics', INTERVAL '365 days', if_not_exists => TRUE);

-- 创建连续聚合视图（用于快速统计查询）
CREATE MATERIALIZED VIEW IF NOT EXISTS hourly_event_stats
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('1 hour', event_time) AS bucket,
    user_id,
    event_type,
    COUNT(*) AS event_count,
    AVG(importance_score) AS avg_importance
FROM timeline_events
GROUP BY bucket, user_id, event_type
WITH NO DATA;

-- 添加刷新策略
SELECT add_continuous_aggregate_policy('hourly_event_stats',
    start_offset => INTERVAL '3 hours',
    end_offset => INTERVAL '1 hour',
    schedule_interval => INTERVAL '1 hour',
    if_not_exists => TRUE
);

-- 创建每日会话统计视图
CREATE MATERIALIZED VIEW IF NOT EXISTS daily_session_stats
WITH (timescaledb.continuous) AS
SELECT
    time_bucket('1 day', message_time) AS bucket,
    user_id,
    session_id,
    COUNT(*) AS message_count,
    SUM(token_count) AS total_tokens
FROM conversation_messages
GROUP BY bucket, user_id, session_id
WITH NO DATA;

-- 添加刷新策略
SELECT add_continuous_aggregate_policy('daily_session_stats',
    start_offset => INTERVAL '3 days',
    end_offset => INTERVAL '1 day',
    schedule_interval => INTERVAL '1 day',
    if_not_exists => TRUE
);

-- 创建触发器函数：自动更新 updated_at 字段
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- 为 timeline_events 表添加触发器
DROP TRIGGER IF EXISTS update_timeline_events_updated_at ON timeline_events;
CREATE TRIGGER update_timeline_events_updated_at
    BEFORE UPDATE ON timeline_events
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

-- 授予权限
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO context_keeper;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO context_keeper;
GRANT ALL PRIVILEGES ON ALL FUNCTIONS IN SCHEMA public TO context_keeper;

-- 输出初始化完成信息
DO $$
BEGIN
    RAISE NOTICE 'TimescaleDB initialization completed successfully!';
    RAISE NOTICE 'Created tables: timeline_events, conversation_messages, context_snapshots, session_statistics';
    RAISE NOTICE 'Created continuous aggregates: hourly_event_stats, daily_session_stats';
    RAISE NOTICE 'Data retention policies: 90-365 days depending on table';
END $$;
