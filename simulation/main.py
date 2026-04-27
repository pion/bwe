#!/usr/bin/env python

# SPDX-FileCopyrightText: 2026 The Pion community <https://pion.ly>
# SPDX-License-Identifier: MIT


import argparse
import plots


def main():
    parser = argparse.ArgumentParser(
            formatter_class=argparse.ArgumentDefaultsHelpFormatter)
    parser.add_argument('-i', '--input',
                        help='input directory containing json files',
                        default='logs')
    parser.add_argument('-o', '--output',
                        help='output directory for generated plot files (png)',
                        default='logs')
    args = parser.parse_args()
    plots.plot_all(args.input, args.output)


if __name__ == "__main__":
    main()
