# SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
# SPDX-License-Identifier: MIT

import pandas as pd
import datetime
import glob
import re
from concurrent.futures import ProcessPoolExecutor

from pathlib import Path

import matplotlib.pyplot as plt
import matplotlib.ticker as mticker


usage_and_state = {
    -1: 'over / decrease',
    0: 'hold / normal',
    1: 'under / increase',
}


def read_json_file(file):
    df = pd.read_json(file, lines=True)
    df['time'] = pd.to_datetime(df['time'], format='mixed')
    df['time'] = df['time'].dt.tz_localize(tz=None)
    df['bits'] = df['payload-size'] * 8

    rtp_tx = df[(df['msg'] == 'rtp') & (df['vantage-point'] ==
                                        'sender')].dropna(axis=1, how='all')
    rtp_rx = df[(df['msg'] == 'rtp') & (df['vantage-point'] ==
                                        'receiver')].dropna(axis=1, how='all')

    latency = rtp_tx.merge(rtp_rx, on='unwrapped-sequence-number')[['time_x',
                                                                    'time_y']]
    latency['latency'] = (latency['time_y'] - latency['time_x']) / \
        datetime.timedelta(milliseconds=1) / 1000.0

    loss = rtp_tx.merge(rtp_rx, on='unwrapped-sequence-number', how='left',
                        indicator=True)
    loss['lost'] = loss['_merge'] == 'left_only'
    loss = loss[['time_x', 'unwrapped-sequence-number', 'lost']]

    p = Path(file)
    return p.stem, df, rtp_tx, rtp_rx, latency, loss


def read_pion_log(file):
    drc_data = []
    drc_pattern = re.compile(r'.*TRACE: (\d{2}:\d{2}:\d{2}\.\d{6}).* ts=(\d{4}/\d{2}/\d{2} \d{2}:\d{2}:\d{2}.\d{6}), seq=(\d+), size=(\d+), interArrivalTime=(\d+), interDepartureTime=(\d+), interGroupDelay=(-?\d+), estimate=(-?\d+.\d+), threshold=(\d+.\d+), usage=(-?\d+), state=(-?\d+)')

    sbwe_data = []
    sbwe_pattern = re.compile(r'.*TRACE: (\d{2}:\d{2}:\d{2}\.\d{6}).* rtt=(\d+), delivered=(\d+), lossTarget=(\d+), delayTarget=(\d+), target=(\d+)')
    with open(file, 'r') as f:
        for line in f:
            match = drc_pattern.match(line)
            if match:
                drc_data.append({
                    'time': pd.to_datetime(f'2000-01-01 {match.group(1)}'),
                    'ts': match.group(2),
                    'seq': int(match.group(3)),
                    'size': int(match.group(4)),
                    'inter_arrival_time': int(match.group(5)),
                    'inter_departure_time': int(match.group(6)),
                    'inter_group_delay': int(match.group(7)),
                    'estimate': float(match.group(8)),
                    'threshold': float(match.group(9)),
                    'usage': int(match.group(10)),
                    'state': int(match.group(11)),
                })
            match = sbwe_pattern.match(line)
            if match:
                sbwe_data.append({
                    'time': pd.to_datetime(f'2000-01-01 {match.group(1)}'),
                    'rtt': int(match.group(2)),
                    'delivered': int(match.group(3)),
                    'loss-target': int(match.group(4)),
                    'delay-target': int(match.group(5)),
                    'target': int(match.group(6)),
                })
    return pd.DataFrame(drc_data), pd.DataFrame(sbwe_data)


def plot_gcc_usage_and_state(ax, df):
    df = df.dropna(subset=['usage', 'state'])
    df['usage'] = -df['usage']
    ax.step(df.index, df['usage'], where='post', label='usage', linewidth=0.5)
    ax.step(df.index, df['state'], where='post', label='state', linewidth=0.5)
    ax.set_xlabel('Time')
    ax.yaxis.set_major_formatter(
        mticker.FuncFormatter(lambda x, pos: usage_and_state.get(x, '')))
    ax.legend(loc='upper right')


def plot_gcc_rtt(ax, df):
    df['rtt'] = df['rtt']*1e-9
    ax.plot(df.index, df['rtt'], label='RTT', linewidth=0.5)
    ax.yaxis.set_major_formatter(mticker.EngFormatter(unit='s'))
    ax.legend(loc='upper right')


def plot_gcc_target_rates(ax, df):
    ax.plot(df.index, df['loss-target'], label='loss-target', linewidth=0.5)
    ax.plot(df.index, df['delay-target'], label='delay-target', linewidth=0.5)
    ax.plot(df.index, df['target'], label='target', linewidth=0.5)
    ax.yaxis.set_major_formatter(mticker.EngFormatter(unit='b/s'))
    ax.legend(loc='upper right')


def plot_gcc_estimates(ax, df):
    df['inter_group_delay'] = df['inter_group_delay'] * 1e-3
    df['estimate'] = df['estimate']
    df['scaled_estimate'] = df['estimate'] * 60
    ax.plot(df.index, df['inter_group_delay'],
            label='inter_group_delay', linewidth=0.5)
    ax.plot(df.index, df['estimate'], label='estimate', linewidth=0.5)
    ax.plot(df.index, df['scaled_estimate'], label='scaled_estimate', linewidth=0.5)
    ax.plot(df.index, df['threshold'], label='threshold', linewidth=0.5)
    ax.plot(df.index, -df['threshold'], label='-threshold', linewidth=0.5)
    ax.yaxis.set_major_formatter(mticker.EngFormatter(unit='s'))
    ax.legend(loc='upper right')


def plot_target_rate(ax, df):
    df = df[df['msg'] == 'setting codec target bitrate']
    ax.plot(df['time'], df['rate'], label='Target Rate', linewidth=0.5)


def plot_rate(ax, label, df):
    df.set_index('time', inplace=True)
    df['bits'] = df['bits'] * 5
    df = df.resample('200ms').sum(numeric_only=True)
    ax.plot(df.index, df['bits'], label=label, linewidth=0.5)


def plot_latency(ax, df):
    ax.plot(df['time_x'], df['latency'], linewidth=0.5)


def plot_loss(ax, df):
    df.set_index('time_x', inplace=True)
    df = df.resample('1s').agg({'lost': 'sum', 'unwrapped-sequence-number':
                                'count'})
    df['ratio'] = df['lost'] / df['unwrapped-sequence-number']
    ax.plot(df.index, df['ratio'], linewidth=0.5)


def plot(output, json, stderr):
    name, df, rtp_tx, rtp_rx, latency, loss = read_json_file(json)
    gcc_drc, gcc_sbwe = read_pion_log(stderr)
    gcc_drc.set_index('time', inplace=True)
    gcc_sbwe.set_index('time', inplace=True)

    fig, ax = plt.subplots(7, 1, sharex=True, figsize=(10, 10),
                           constrained_layout=True)

    plot_rate(ax[0], 'Send Rate', rtp_tx)
    plot_rate(ax[0], 'Receive Rate', rtp_rx)
    plot_target_rate(ax[0], df)
    ax[0].set_title('RTP Rates')
    ax[0].yaxis.set_major_formatter(mticker.EngFormatter(unit='b/s'))
    ax[0].legend(loc='upper right')

    plot_gcc_target_rates(ax[1], gcc_sbwe)
    ax[1].set_title('GCC Target Rates')

    plot_latency(ax[2], latency)
    ax[2].set_title('E2E Delay')
    ax[2].yaxis.set_major_formatter(mticker.EngFormatter(unit='s'))

    plot_gcc_estimates(ax[3], gcc_drc)
    ax[3].set_title('GCC Estimates')

    plot_gcc_usage_and_state(ax[4], gcc_drc)
    ax[4].set_title('GCC Usage and State')

    plot_gcc_rtt(ax[5], gcc_sbwe)
    ax[5].set_title('GCC RTT')

    plot_loss(ax[6], loss)
    ax[6].set_title('Packet Loss')
    ax[6].yaxis.set_major_formatter(mticker.PercentFormatter(xmax=1.0))

    fig.suptitle(name)
    plt.savefig(f'{output}/{name}.png', dpi=450)
    plt.close(fig)


def plot_all(input, output):
    json_logs = sorted(glob.glob(f'{input}/*.jsonl'))
    stderr_logs = sorted(glob.glob(f'{input}/*.stderr'))
    with ProcessPoolExecutor() as executor:
        results = list(executor.map(plot, [output] * len(json_logs), json_logs, stderr_logs))
